package remotewritebench

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"

	"github.com/ClarkElll/my-tools/internal/logutil"
	"github.com/ClarkElll/my-tools/internal/schedutil"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type observedRequest struct {
	path    string
	headers http.Header
	write   prompb.WriteRequest
}

func TestConfigNormalizedDefaultsMaxSeriesPerRequest(t *testing.T) {
	cfg, err := Config{
		URL:             "http://localhost:9090/api/v1/write",
		Series:          10,
		RequestInterval: time.Second,
		SampleInterval:  time.Millisecond,
		Concurrency:     3,
		Timeout:         time.Second,
	}.Normalized()
	if err != nil {
		t.Fatalf("Normalized returned error: %v", err)
	}

	if cfg.MaxSeriesPerRequest != 10 {
		t.Fatalf("expected max series per request to default to series count, got %d", cfg.MaxSeriesPerRequest)
	}
	if cfg.StartTime.IsZero() {
		t.Fatalf("expected normalized start time to be set")
	}
}

func TestConfigNormalizedRejectsSampleIntervalBelowMillisecond(t *testing.T) {
	_, err := Config{
		URL:             "http://localhost:9090/api/v1/write",
		Series:          1,
		RequestInterval: time.Second,
		SampleInterval:  500 * time.Microsecond,
		Concurrency:     1,
		Timeout:         time.Second,
	}.Normalized()
	if err == nil || !strings.Contains(err.Error(), "at least 1ms") {
		t.Fatalf("expected sample-interval validation error, got %v", err)
	}
}

func TestBuildSeriesStatesAddsUTF8LabelWhenEnabled(t *testing.T) {
	cfg, err := Config{
		URL:             "http://localhost:9090/api/v1/write",
		Series:          2,
		RequestInterval: time.Second,
		SampleInterval:  time.Millisecond,
		Concurrency:     1,
		Timeout:         time.Second,
		UTF8Label:       true,
	}.Normalized()
	if err != nil {
		t.Fatalf("Normalized returned error: %v", err)
	}

	states, err := buildSeriesStates(cfg)
	if err != nil {
		t.Fatalf("buildSeriesStates returned error: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("expected 2 series states, got %d", len(states))
	}

	for index, state := range states {
		labels := labelsToMap(state.labels)
		if labels["__name__"] != DefaultMetricName {
			t.Fatalf("expected metric name %q, got %q", DefaultMetricName, labels["__name__"])
		}
		if labels["series_id"] != fmt.Sprintf("series_id-%0*d", 1, index) {
			t.Fatalf("expected series_id label for series %d, got %q", index, labels["series_id"])
		}
		if labels[UTF8LabelName] != UTF8LabelValue {
			t.Fatalf("expected UTF-8 label %q=%q, got %v", UTF8LabelName, UTF8LabelValue, labels)
		}
	}
}

func TestBuildSeriesStatesZeroPadsSeriesIDForLexicographicOrder(t *testing.T) {
	cfg, err := Config{
		URL:             "http://localhost:9090/api/v1/write",
		Series:          12,
		RequestInterval: time.Second,
		SampleInterval:  time.Millisecond,
		Concurrency:     1,
		Timeout:         time.Second,
	}.Normalized()
	if err != nil {
		t.Fatalf("Normalized returned error: %v", err)
	}

	states, err := buildSeriesStates(cfg)
	if err != nil {
		t.Fatalf("buildSeriesStates returned error: %v", err)
	}

	got := make([]string, 0, len(states))
	for _, state := range states {
		got = append(got, labelsToMap(state.labels)["series_id"])
	}

	want := []string{
		"series_id-00",
		"series_id-01",
		"series_id-02",
		"series_id-03",
		"series_id-04",
		"series_id-05",
		"series_id-06",
		"series_id-07",
		"series_id-08",
		"series_id-09",
		"series_id-10",
		"series_id-11",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("expected zero-padded series ids %v, got %v", want, got)
	}
}

func TestCycleRunnerSendsRemoteWriteRequest(t *testing.T) {
	cfg, err := Config{
		URL:                 "http://example.com/api/v1/write",
		Series:              2,
		RequestInterval:     time.Second,
		SampleInterval:      15 * time.Second,
		Concurrency:         1,
		MaxSeriesPerRequest: 2,
		Timeout:             time.Second,
		StartTime:           time.UnixMilli(1_700_000_000_000),
	}.Normalized()
	if err != nil {
		t.Fatalf("Normalized returned error: %v", err)
	}

	states, err := buildSeriesStates(cfg)
	if err != nil {
		t.Fatalf("buildSeriesStates returned error: %v", err)
	}

	observed := make([]observedRequest, 0, 1)
	var logbuf bytes.Buffer
	stats := &statsRecorder{}
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			writeRequest := decodeWriteRequest(t, r)
			observed = append(observed, observedRequest{
				path:    r.URL.Path,
				headers: r.Header.Clone(),
				write:   writeRequest,
			})

			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	runner := cycleRunner{
		cfg:           cfg,
		client:        client,
		url:           cfg.URL,
		logger:        logutil.New(&logbuf, logutil.Options{}),
		stats:         stats,
		batches:       splitSeriesStates(states, cfg.MaxSeriesPerRequest),
		baseTimestamp: cfg.StartTime.UnixMilli(),
	}

	runner.run(context.Background(), schedutil.Job{Sequence: 0, ScheduledAt: cfg.StartTime})

	if len(observed) != 1 {
		t.Fatalf("expected 1 observed request, got %d", len(observed))
	}
	if observed[0].path != "/api/v1/write" {
		t.Fatalf("expected request path /api/v1/write, got %s", observed[0].path)
	}
	if got := observed[0].headers.Get("Content-Encoding"); got != "snappy" {
		t.Fatalf("expected Content-Encoding snappy, got %q", got)
	}
	if got := observed[0].headers.Get("Content-Type"); got != "application/x-protobuf" {
		t.Fatalf("expected protobuf Content-Type, got %q", got)
	}
	if got := observed[0].headers.Get("X-Prometheus-Remote-Write-Version"); got != remoteWriteVersion {
		t.Fatalf("expected remote write version %q, got %q", remoteWriteVersion, got)
	}

	if len(observed[0].write.Timeseries) != 2 {
		t.Fatalf("expected 2 time series, got %d", len(observed[0].write.Timeseries))
	}
	for _, series := range observed[0].write.Timeseries {
		if len(series.Samples) != 1 {
			t.Fatalf("expected 1 sample per series, got %d", len(series.Samples))
		}
		if len(series.Labels) == 0 || series.Labels[0].Name != "__name__" {
			t.Fatalf("expected sorted labels with __name__ first, got %+v", series.Labels)
		}
		if got := series.Samples[0].Timestamp; got != cfg.StartTime.UnixMilli() {
			t.Fatalf("expected timestamp %d, got %d", cfg.StartTime.UnixMilli(), got)
		}
	}

	logOutput := logbuf.String()
	if !strings.Contains(logOutput, "msg=\"remote-write request succeeded\"") {
		t.Fatalf("expected success log entry, got %q", logOutput)
	}
	if !strings.Contains(logOutput, "status=\"204 No Content\"") {
		t.Fatalf("expected success status in logs, got %q", logOutput)
	}
	if !strings.Contains(logOutput, "series=2") || !strings.Contains(logOutput, "samples=2") {
		t.Fatalf("expected request counters in logs, got %q", logOutput)
	}

	snapshot := stats.snapshot()
	if snapshot.Requests != 1 || snapshot.FailedRequests != 0 {
		t.Fatalf("unexpected stats snapshot: %+v", snapshot)
	}
	if snapshot.SeriesSent != 2 || snapshot.SamplesSent != 2 {
		t.Fatalf("unexpected success counters: %+v", snapshot)
	}
	if snapshot.PayloadBytes == 0 {
		t.Fatalf("expected payload bytes to be recorded, got %+v", snapshot)
	}
}

func TestCycleRunnerSplitsRequestsByMaxSeriesPerRequest(t *testing.T) {
	cfg, err := Config{
		URL:                 "http://example.com/api/v1/write",
		Series:              5,
		RequestInterval:     time.Second,
		SampleInterval:      15 * time.Second,
		Concurrency:         1,
		MaxSeriesPerRequest: 2,
		Timeout:             time.Second,
		StartTime:           time.UnixMilli(1_700_000_000_000),
	}.Normalized()
	if err != nil {
		t.Fatalf("Normalized returned error: %v", err)
	}

	states, err := buildSeriesStates(cfg)
	if err != nil {
		t.Fatalf("buildSeriesStates returned error: %v", err)
	}

	requestSizes := make([]int, 0, 3)
	stats := &statsRecorder{}
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			writeRequest := decodeWriteRequest(t, r)
			requestSizes = append(requestSizes, len(writeRequest.Timeseries))
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	runner := cycleRunner{
		cfg:           cfg,
		client:        client,
		url:           cfg.URL,
		logger:        logutil.New(io.Discard, logutil.Options{}),
		stats:         stats,
		batches:       splitSeriesStates(states, cfg.MaxSeriesPerRequest),
		baseTimestamp: cfg.StartTime.UnixMilli(),
	}

	runner.run(context.Background(), schedutil.Job{Sequence: 0, ScheduledAt: cfg.StartTime})

	if got, want := len(requestSizes), 3; got != want {
		t.Fatalf("expected %d requests, got %d", want, got)
	}
	if requestSizes[0] != 2 || requestSizes[1] != 2 || requestSizes[2] != 1 {
		t.Fatalf("expected request sizes [2 2 1], got %v", requestSizes)
	}

	snapshot := stats.snapshot()
	if snapshot.Requests != 3 || snapshot.FailedRequests != 0 {
		t.Fatalf("unexpected stats snapshot: %+v", snapshot)
	}
	if snapshot.SeriesSent != 5 || snapshot.SamplesSent != 5 {
		t.Fatalf("unexpected success counters: %+v", snapshot)
	}
}

func TestBuildRequestPayloadUsesSampleIndexes(t *testing.T) {
	cfg, err := Config{
		URL:             "http://example.com/api/v1/write",
		Series:          1,
		RequestInterval: 15 * time.Second,
		SampleInterval:  15 * time.Second,
		Concurrency:     1,
		Timeout:         time.Second,
	}.Normalized()
	if err != nil {
		t.Fatalf("Normalized returned error: %v", err)
	}

	states, err := buildSeriesStates(cfg)
	if err != nil {
		t.Fatalf("buildSeriesStates returned error: %v", err)
	}

	payload, seriesCount, sampleCount, err := buildRequestPayload(states, 1_700_000_000_000, 2, 2, cfg.SampleInterval)
	if err != nil {
		t.Fatalf("buildRequestPayload returned error: %v", err)
	}
	if seriesCount != 1 || sampleCount != 1 {
		t.Fatalf("unexpected payload counters: series=%d samples=%d", seriesCount, sampleCount)
	}

	var writeRequest prompb.WriteRequest
	data, err := snappy.Decode(nil, payload)
	if err != nil {
		t.Fatalf("snappy decode returned error: %v", err)
	}
	if err := writeRequest.Unmarshal(data); err != nil {
		t.Fatalf("WriteRequest.Unmarshal returned error: %v", err)
	}

	got := writeRequest.Timeseries[0].Samples[0]
	if want := int64(1_700_000_030_000); got.Timestamp != want {
		t.Fatalf("expected timestamp %d, got %d", want, got.Timestamp)
	}
	if want := float64(3); got.Value != want {
		t.Fatalf("expected value %v, got %v", want, got.Value)
	}
}

func TestCycleRunnerBatchesMultipleSamplesPerRequest(t *testing.T) {
	cfg, err := Config{
		URL:                 "http://example.com/api/v1/write",
		Series:              2,
		RequestInterval:     15 * time.Second,
		SampleInterval:      5 * time.Second,
		Concurrency:         1,
		MaxSeriesPerRequest: 2,
		Timeout:             time.Second,
		StartTime:           time.UnixMilli(1_700_000_000_000),
	}.Normalized()
	if err != nil {
		t.Fatalf("Normalized returned error: %v", err)
	}

	states, err := buildSeriesStates(cfg)
	if err != nil {
		t.Fatalf("buildSeriesStates returned error: %v", err)
	}

	observed := make([]observedRequest, 0, 1)
	stats := &statsRecorder{}
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			observed = append(observed, observedRequest{
				path:    r.URL.Path,
				headers: r.Header.Clone(),
				write:   decodeWriteRequest(t, r),
			})
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	runner := cycleRunner{
		cfg:           cfg,
		client:        client,
		url:           cfg.URL,
		logger:        logutil.New(io.Discard, logutil.Options{}),
		stats:         stats,
		batches:       splitSeriesStates(states, cfg.MaxSeriesPerRequest),
		baseTimestamp: cfg.StartTime.UnixMilli(),
	}

	runner.run(context.Background(), schedutil.Job{Sequence: 1, ScheduledAt: cfg.StartTime.Add(cfg.RequestInterval)})

	if len(observed) != 1 {
		t.Fatalf("expected 1 observed request, got %d", len(observed))
	}
	if len(observed[0].write.Timeseries) != 2 {
		t.Fatalf("expected 2 time series, got %d", len(observed[0].write.Timeseries))
	}
	for _, series := range observed[0].write.Timeseries {
		if len(series.Samples) != 3 {
			t.Fatalf("expected 3 samples per series, got %d", len(series.Samples))
		}
		for sampleIndex, sample := range series.Samples {
			wantTS := cfg.StartTime.UnixMilli() + int64((sampleIndex+1)*5_000)
			if sample.Timestamp != wantTS {
				t.Fatalf("expected timestamp %d, got %d", wantTS, sample.Timestamp)
			}
			wantValue := float64(sampleIndex + 2)
			if sample.Value != wantValue {
				t.Fatalf("expected value %v, got %v", wantValue, sample.Value)
			}
		}
	}

	snapshot := stats.snapshot()
	if snapshot.Requests != 1 || snapshot.FailedRequests != 0 {
		t.Fatalf("unexpected stats snapshot: %+v", snapshot)
	}
	if snapshot.SeriesSent != 2 || snapshot.SamplesSent != 6 {
		t.Fatalf("unexpected success counters: %+v", snapshot)
	}
}

func TestCycleRunnerSkipsRequestWhenNoNewSamplesAreDue(t *testing.T) {
	cfg, err := Config{
		URL:                 "http://example.com/api/v1/write",
		Series:              2,
		RequestInterval:     5 * time.Second,
		SampleInterval:      15 * time.Second,
		Concurrency:         1,
		MaxSeriesPerRequest: 2,
		Timeout:             time.Second,
		StartTime:           time.UnixMilli(1_700_000_000_000),
	}.Normalized()
	if err != nil {
		t.Fatalf("Normalized returned error: %v", err)
	}

	states, err := buildSeriesStates(cfg)
	if err != nil {
		t.Fatalf("buildSeriesStates returned error: %v", err)
	}

	requests := 0
	stats := &statsRecorder{}
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	runner := cycleRunner{
		cfg:           cfg,
		client:        client,
		url:           cfg.URL,
		logger:        logutil.New(io.Discard, logutil.Options{}),
		stats:         stats,
		batches:       splitSeriesStates(states, cfg.MaxSeriesPerRequest),
		baseTimestamp: cfg.StartTime.UnixMilli(),
	}

	runner.run(context.Background(), schedutil.Job{Sequence: 1, ScheduledAt: cfg.StartTime.Add(cfg.RequestInterval)})

	if requests != 0 {
		t.Fatalf("expected no requests when no new samples are due, got %d", requests)
	}

	snapshot := stats.snapshot()
	if snapshot.Requests != 0 || snapshot.FailedRequests != 0 || snapshot.SeriesSent != 0 || snapshot.SamplesSent != 0 {
		t.Fatalf("expected empty stats snapshot, got %+v", snapshot)
	}
}

func TestCycleRunnerRecordsNon2xxFailure(t *testing.T) {
	cfg, err := Config{
		URL:                 "http://example.com/api/v1/write",
		Series:              1,
		RequestInterval:     time.Second,
		SampleInterval:      time.Second,
		Concurrency:         1,
		MaxSeriesPerRequest: 1,
		Timeout:             time.Second,
	}.Normalized()
	if err != nil {
		t.Fatalf("Normalized returned error: %v", err)
	}

	states, err := buildSeriesStates(cfg)
	if err != nil {
		t.Fatalf("buildSeriesStates returned error: %v", err)
	}

	var logbuf bytes.Buffer
	stats := &statsRecorder{}
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Status:     "500 Internal Server Error",
				Body:       io.NopCloser(strings.NewReader("boom")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	runner := cycleRunner{
		cfg:           cfg,
		client:        client,
		url:           cfg.URL,
		logger:        logutil.New(&logbuf, logutil.Options{}),
		stats:         stats,
		batches:       splitSeriesStates(states, cfg.MaxSeriesPerRequest),
		baseTimestamp: cfg.StartTime.UnixMilli(),
	}

	runner.run(context.Background(), schedutil.Job{Sequence: 0, ScheduledAt: cfg.StartTime})

	logOutput := logbuf.String()
	if !strings.Contains(logOutput, "msg=\"remote-write request failed\"") {
		t.Fatalf("expected failure log entry, got %q", logOutput)
	}
	if !strings.Contains(logOutput, "status=\"500 Internal Server Error\"") {
		t.Fatalf("expected failure status in logs, got %q", logOutput)
	}
	if !strings.Contains(logOutput, "series=1") || !strings.Contains(logOutput, "samples=1") {
		t.Fatalf("expected request counters in failure logs, got %q", logOutput)
	}

	snapshot := stats.snapshot()
	if snapshot.Requests != 1 || snapshot.FailedRequests != 1 {
		t.Fatalf("unexpected failure snapshot: %+v", snapshot)
	}
	if snapshot.SeriesSent != 0 || snapshot.SamplesSent != 0 {
		t.Fatalf("failed request should not increment success counters: %+v", snapshot)
	}
	if snapshot.PayloadBytes == 0 {
		t.Fatalf("expected payload bytes for attempted request, got %+v", snapshot)
	}
}

func decodeWriteRequest(t *testing.T, r *http.Request) prompb.WriteRequest {
	t.Helper()

	compressed, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}

	payload, err := snappy.Decode(nil, compressed)
	if err != nil {
		t.Fatalf("snappy decode returned error: %v", err)
	}

	var writeRequest prompb.WriteRequest
	if err := writeRequest.Unmarshal(payload); err != nil {
		t.Fatalf("WriteRequest.Unmarshal returned error: %v", err)
	}

	return writeRequest
}

func labelsToMap(labels []prompb.Label) map[string]string {
	mapped := make(map[string]string, len(labels))
	for _, label := range labels {
		mapped[label.Name] = label.Value
	}

	return mapped
}
