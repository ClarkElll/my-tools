package remotewritebench

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestBenchMetricsWritePrometheus(t *testing.T) {
	m := newBenchMetrics(Config{
		Series:              3,
		Concurrency:         2,
		MaxSeriesPerRequest: 2,
		RequestInterval:     time.Second,
		SampleInterval:      15 * time.Second,
		UTF8Label:           true,
	})

	m.runStarted()
	m.requestStarted()
	m.recordSuccess(150*time.Millisecond, 2, 4, 300)
	m.requestFinished()
	m.requestStarted()
	m.recordFailure(50*time.Millisecond, 40)
	m.requestFinished()
	m.runCompleted()

	var buf bytes.Buffer
	m.metricSet().WritePrometheus(&buf)

	output := buf.String()
	checks := []string{
		"my_tools_remote_write_bench_running 0",
		"my_tools_remote_write_bench_inflight_requests 0",
		"my_tools_remote_write_bench_configured_series 3",
		"my_tools_remote_write_bench_configured_concurrency 2",
		"my_tools_remote_write_bench_configured_max_series_per_request 2",
		"my_tools_remote_write_bench_configured_request_interval_seconds 1",
		"my_tools_remote_write_bench_configured_sample_interval_seconds 15",
		"my_tools_remote_write_bench_configured_utf8_label 1",
		"my_tools_remote_write_bench_requests_total 2",
		"my_tools_remote_write_bench_request_failures_total 1",
		"my_tools_remote_write_bench_samples_sent_total 4",
		"my_tools_remote_write_bench_series_sent_total 2",
		"my_tools_remote_write_bench_payload_bytes_total 340",
		"my_tools_remote_write_bench_request_duration_seconds_count 2",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Fatalf("expected metrics output to contain %q, got %q", check, output)
		}
	}
}
