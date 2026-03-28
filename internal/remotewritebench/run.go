package remotewritebench

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"

	"github.com/ClarkElll/my-tools/internal/schedutil"
)

const remoteWriteVersion = "0.1.0"

type statsRecorder struct {
	requests       atomic.Int64
	failedRequests atomic.Int64
	seriesSent     atomic.Int64
	samplesSent    atomic.Int64
	payloadBytes   atomic.Int64
	latencyTotalNS atomic.Int64
	latencyMaxNS   atomic.Int64
	metrics        *benchMetrics
}

type statsSnapshot struct {
	Requests       int64
	FailedRequests int64
	SeriesSent     int64
	SamplesSent    int64
	PayloadBytes   int64
	LatencyTotalNS int64
	LatencyMaxNS   int64
}

type cycleRunner struct {
	cfg           Config
	client        *http.Client
	url           string
	logger        *slog.Logger
	stats         *statsRecorder
	batches       [][]*seriesState
	baseTimestamp int64
	pool          *realisticPool // non-nil in realistic mode
}

func Run(ctx context.Context, logger *slog.Logger, cfg Config) error {
	normalizedCfg, err := cfg.Normalized()
	if err != nil {
		return err
	}
	cfg = normalizedCfg
	logger = normalizeLogger(logger)

	if cfg.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Duration)
		defer cancel()
	}

	var (
		seriesStates []*seriesState
		pool         *realisticPool
	)
	if cfg.Realistic {
		pool, err = buildRealisticSeriesStates(cfg)
		if err != nil {
			return err
		}
		seriesStates = pool.allStates()
	} else {
		seriesStates, err = buildSeriesStates(cfg)
		if err != nil {
			return err
		}
	}

	benchMetrics := newBenchMetrics(cfg)
	benchMetrics.runStarted()
	defer benchMetrics.runCompleted()

	stats := &statsRecorder{metrics: benchMetrics}
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			MaxIdleConns:        cfg.Concurrency * 4,
			MaxIdleConnsPerHost: cfg.Concurrency * 4,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	logger.Info("starting remote-write-bench",
		"url", cfg.URL,
		"metric", DefaultMetricName,
		"configured_series", cfg.Series,
		"max_series_per_request", cfg.MaxSeriesPerRequest,
		"concurrency", cfg.Concurrency,
		"request_interval", cfg.RequestInterval,
		"sample_interval", cfg.SampleInterval,
		"duration", formatOptionalDuration(cfg.Duration),
		"timeout", cfg.Timeout,
		"utf8_label", cfg.UTF8Label,
		"realistic", cfg.Realistic,
	)

	startedAt := time.Now()
	runner := cycleRunner{
		cfg:           cfg,
		client:        client,
		url:           cfg.URL,
		logger:        logger,
		stats:         stats,
		batches:       splitSeriesStates(seriesStates, cfg.MaxSeriesPerRequest),
		baseTimestamp: cfg.StartTime.UnixMilli(),
		pool:          pool,
	}

	if err := schedutil.Run(ctx, schedutil.Options{
		Interval:      cfg.RequestInterval,
		Workers:       cfg.Concurrency,
		QueueCapacity: 16,
		Immediate:     true,
		Run:           runner.run,
	}); err != nil {
		return err
	}

	snapshot := stats.snapshot()
	logger.Info("remote-write-bench completed",
		"elapsed", time.Since(startedAt).Round(time.Millisecond),
		"configured_series", cfg.Series,
		"requests", snapshot.Requests,
		"successful_requests", snapshot.Requests-snapshot.FailedRequests,
		"failed_requests", snapshot.FailedRequests,
		"series_sent_total", snapshot.SeriesSent,
		"samples_sent_total", snapshot.SamplesSent,
		"payload_bytes", snapshot.PayloadBytes,
		"avg_latency", averageLatency(snapshot),
		"max_latency", time.Duration(snapshot.LatencyMaxNS),
	)

	return nil
}

func splitSeriesStates(states []*seriesState, batchSize int) [][]*seriesState {
	if len(states) == 0 {
		return nil
	}

	batches := make([][]*seriesState, 0, (len(states)+batchSize-1)/batchSize)
	for start := 0; start < len(states); start += batchSize {
		end := min(start+batchSize, len(states))
		batches = append(batches, states[start:end])
	}

	return batches
}

func (r cycleRunner) run(ctx context.Context, job schedutil.Job) {
	batches := r.batches
	if r.pool != nil {
		r.pool.maybeChurn(job.Sequence)
		batches = splitSeriesStates(r.pool.allStates(), r.cfg.MaxSeriesPerRequest)
	}

	for _, batch := range batches {
		if ctx.Err() != nil {
			return
		}

		r.sendBatch(ctx, batch, job.Sequence)
	}
}

func (r cycleRunner) sendBatch(ctx context.Context, batch []*seriesState, sequence int64) {
	firstSampleIndex, lastSampleIndex, ok := sampleRangeForSequence(sequence, r.cfg.RequestInterval, r.cfg.SampleInterval)
	if !ok {
		return
	}

	r.stats.requestStarted()
	defer r.stats.requestFinished()

	payload, seriesCount, sampleCount, err := buildRequestPayload(batch, r.baseTimestamp, firstSampleIndex, lastSampleIndex, r.cfg.SampleInterval)
	if err != nil {
		r.stats.recordFailure(time.Duration(0), 0)
		r.logger.Error("remote-write encode failed", "err", err)
		return
	}

	requestCtx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, r.url, bytes.NewReader(payload))
	if err != nil {
		r.stats.recordFailure(time.Duration(0), 0)
		r.logger.Error("remote-write request creation failed", "err", err)
		return
	}

	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("X-Prometheus-Remote-Write-Version", remoteWriteVersion)

	startedAt := time.Now()
	resp, err := r.client.Do(req)
	latency := time.Since(startedAt)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		r.stats.recordFailure(latency, len(payload))
		r.logger.Error("remote-write request failed",
			"sequence", sequence,
			"series", seriesCount,
			"samples", sampleCount,
			"payload_bytes", len(payload),
			"latency", latency,
			"err", err,
		)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		r.stats.recordFailure(latency, len(payload))
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		r.logger.Error("remote-write request failed",
			"sequence", sequence,
			"series", seriesCount,
			"samples", sampleCount,
			"payload_bytes", len(payload),
			"latency", latency,
			"status", responseStatus(resp),
			"body", strings.TrimSpace(string(body)),
		)
		return
	}

	io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	r.stats.recordSuccess(latency, seriesCount, sampleCount, len(payload))
	r.logger.Info("remote-write request succeeded",
		"sequence", sequence,
		"series", seriesCount,
		"samples", sampleCount,
		"payload_bytes", len(payload),
		"latency", latency,
		"status", responseStatus(resp),
	)
}

func buildRequestPayload(batch []*seriesState, baseTimestamp int64, firstSampleIndex int64, lastSampleIndex int64, sampleInterval time.Duration) ([]byte, int, int, error) {
	samplesPerSeries := int(lastSampleIndex-firstSampleIndex) + 1
	if samplesPerSeries <= 0 {
		return nil, len(batch), 0, nil
	}

	timeSeries := make([]prompb.TimeSeries, 0, len(batch))
	sampleIntervalMS := sampleInterval.Milliseconds()

	for _, state := range batch {
		samples := make([]prompb.Sample, 0, samplesPerSeries)
		for sampleIndex := firstSampleIndex; sampleIndex <= lastSampleIndex; sampleIndex++ {
			samples = append(samples, prompb.Sample{
				Timestamp: baseTimestamp + sampleIndex*sampleIntervalMS,
				Value:     float64(sampleIndex + 1),
			})
		}

		timeSeries = append(timeSeries, prompb.TimeSeries{
			Labels:  slices.Clone(state.labels),
			Samples: samples,
		})
	}

	writeRequest := &prompb.WriteRequest{Timeseries: timeSeries}
	data, err := writeRequest.Marshal()
	if err != nil {
		return nil, 0, 0, err
	}

	return snappy.Encode(nil, data), len(batch), len(batch) * samplesPerSeries, nil
}

func sampleRangeForSequence(sequence int64, requestInterval time.Duration, sampleInterval time.Duration) (int64, int64, bool) {
	previousTotal := samplesAvailableBySequence(sequence-1, requestInterval, sampleInterval)
	currentTotal := samplesAvailableBySequence(sequence, requestInterval, sampleInterval)
	if currentTotal <= previousTotal {
		return 0, 0, false
	}

	return previousTotal, currentTotal - 1, true
}

func samplesAvailableBySequence(sequence int64, requestInterval time.Duration, sampleInterval time.Duration) int64 {
	if sequence < 0 {
		return 0
	}

	elapsedMS := sequence * requestInterval.Milliseconds()
	return elapsedMS/sampleInterval.Milliseconds() + 1
}

func (s *statsRecorder) recordSuccess(latency time.Duration, seriesCount int, sampleCount int, payloadBytes int) {
	s.requests.Add(1)
	s.seriesSent.Add(int64(seriesCount))
	s.samplesSent.Add(int64(sampleCount))
	s.payloadBytes.Add(int64(payloadBytes))
	s.addLatency(latency)
	if s.metrics != nil {
		s.metrics.recordSuccess(latency, seriesCount, sampleCount, payloadBytes)
	}
}

func (s *statsRecorder) recordFailure(latency time.Duration, payloadBytes int) {
	s.requests.Add(1)
	s.failedRequests.Add(1)
	s.payloadBytes.Add(int64(payloadBytes))
	s.addLatency(latency)
	if s.metrics != nil {
		s.metrics.recordFailure(latency, payloadBytes)
	}
}

func (s *statsRecorder) addLatency(latency time.Duration) {
	s.latencyTotalNS.Add(latency.Nanoseconds())
	latencyNS := latency.Nanoseconds()
	for {
		currentMax := s.latencyMaxNS.Load()
		if latencyNS <= currentMax {
			return
		}
		if s.latencyMaxNS.CompareAndSwap(currentMax, latencyNS) {
			return
		}
	}
}

func (s *statsRecorder) snapshot() statsSnapshot {
	return statsSnapshot{
		Requests:       s.requests.Load(),
		FailedRequests: s.failedRequests.Load(),
		SeriesSent:     s.seriesSent.Load(),
		SamplesSent:    s.samplesSent.Load(),
		PayloadBytes:   s.payloadBytes.Load(),
		LatencyTotalNS: s.latencyTotalNS.Load(),
		LatencyMaxNS:   s.latencyMaxNS.Load(),
	}
}

func averageLatency(snapshot statsSnapshot) time.Duration {
	if snapshot.Requests == 0 {
		return 0
	}

	return time.Duration(snapshot.LatencyTotalNS / snapshot.Requests)
}

func (s *statsRecorder) requestStarted() {
	if s.metrics != nil {
		s.metrics.requestStarted()
	}
}

func (s *statsRecorder) requestFinished() {
	if s.metrics != nil {
		s.metrics.requestFinished()
	}
}

func normalizeLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}

	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func formatOptionalDuration(value time.Duration) string {
	if value == 0 {
		return "until-interrupted"
	}
	return value.String()
}

func responseStatus(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	if strings.TrimSpace(resp.Status) != "" {
		return resp.Status
	}

	code := strconv.Itoa(resp.StatusCode)
	text := http.StatusText(resp.StatusCode)
	if text == "" {
		return code
	}

	return code + " " + text
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
