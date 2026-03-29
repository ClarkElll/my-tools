package querybench

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/ClarkElll/my-tools/internal/metricsutil"
	"github.com/ClarkElll/my-tools/internal/schedutil"
)

type statsRecorder struct {
	requests       atomic.Int64
	failedRequests atomic.Int64
	latencyTotalNS atomic.Int64
	latencyMaxNS   atomic.Int64
}

func (s *statsRecorder) recordSuccess(latency time.Duration) {
	s.requests.Add(1)
	s.addLatency(latency)
}

func (s *statsRecorder) recordFailure(latency time.Duration, _ string) {
	s.requests.Add(1)
	s.failedRequests.Add(1)
	if latency > 0 {
		s.addLatency(latency)
	}
}

func (s *statsRecorder) addLatency(latency time.Duration) {
	ns := latency.Nanoseconds()
	s.latencyTotalNS.Add(ns)
	for {
		old := s.latencyMaxNS.Load()
		if ns <= old || s.latencyMaxNS.CompareAndSwap(old, ns) {
			break
		}
	}
}

type snapshot struct {
	Requests       int64
	FailedRequests int64
	LatencyTotalNS int64
	LatencyMaxNS   int64
}

func (s *statsRecorder) snapshot() snapshot {
	return snapshot{
		Requests:       s.requests.Load(),
		FailedRequests: s.failedRequests.Load(),
		LatencyTotalNS: s.latencyTotalNS.Load(),
		LatencyMaxNS:   s.latencyMaxNS.Load(),
	}
}

func Run(ctx context.Context, logger *slog.Logger, cfg Config) error {
	normalizedCfg, err := cfg.Normalized()
	if err != nil {
		return err
	}
	cfg = normalizedCfg

	if logger == nil {
		logger = slog.Default()
	}

	if cfg.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Duration)
		defer cancel()
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			MaxIdleConns:        cfg.Concurrency * 4,
			MaxIdleConnsPerHost: cfg.Concurrency * 4,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	stats := &statsRecorder{}

	if _, err := metricsutil.Start(ctx, logger, metricsutil.Config{ListenAddr: cfg.ListenAddr}); err != nil {
		return err
	}

	q := &querier{
		cfg:    cfg,
		client: client,
		logger: logger,
		stats:  stats,
	}

	logger.Info("starting query-bench",
		"url", cfg.URL,
		"endpoint", cfg.Endpoint,
		"query", cfg.Query,
		"qps", cfg.QPS,
		"concurrency", cfg.Concurrency,
		"timeout", cfg.Timeout,
		"duration", formatOptionalDuration(cfg.Duration),
	)

	startedAt := time.Now()

	if err := schedutil.Run(ctx, schedutil.Options{
		Interval:      cfg.Interval(),
		Workers:       cfg.Concurrency,
		QueueCapacity: 16,
		Immediate:     true,
		Run:           q.run,
	}); err != nil {
		return err
	}

	snap := stats.snapshot()
	avgLatency := time.Duration(0)
	successful := snap.Requests - snap.FailedRequests
	if successful > 0 {
		avgLatency = time.Duration(snap.LatencyTotalNS / successful)
	}

	logger.Info("query-bench completed",
		"elapsed", time.Since(startedAt).Round(time.Millisecond),
		"requests", snap.Requests,
		"failed", snap.FailedRequests,
		"avg_latency", avgLatency,
		"max_latency", time.Duration(snap.LatencyMaxNS),
	)

	return nil
}

func formatOptionalDuration(d time.Duration) string {
	if d == 0 {
		return "unlimited"
	}
	return d.String()
}
