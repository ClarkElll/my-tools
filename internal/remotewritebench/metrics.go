package remotewritebench

import (
	"time"

	vmmetrics "github.com/VictoriaMetrics/metrics"
)

const benchMetricPrefix = "my_tools_remote_write_bench_"

type benchMetrics struct {
	set *vmmetrics.Set

	running                   *vmmetrics.Gauge
	inflightRequests          *vmmetrics.Gauge
	lastRequestLatencySeconds *vmmetrics.Gauge

	configuredSeries                *vmmetrics.Gauge
	configuredConcurrency           *vmmetrics.Gauge
	configuredMaxSeriesPerRequest   *vmmetrics.Gauge
	configuredRequestInterval       *vmmetrics.Gauge
	configuredSampleInterval        *vmmetrics.Gauge
	configuredUTF8Label             *vmmetrics.Gauge
	requestsTotal                   *vmmetrics.Counter
	requestFailuresTotal            *vmmetrics.Counter
	samplesSentTotal                *vmmetrics.Counter
	seriesSentTotal                 *vmmetrics.Counter
	payloadBytesTotal               *vmmetrics.Counter
	requestDurationSecondsHistogram *vmmetrics.PrometheusHistogram
}

func newBenchMetrics(cfg Config) *benchMetrics {
	set := vmmetrics.NewSet()
	m := &benchMetrics{
		set: set,

		running:                   set.NewGauge(benchMetricPrefix+"running", nil),
		inflightRequests:          set.NewGauge(benchMetricPrefix+"inflight_requests", nil),
		lastRequestLatencySeconds: set.NewGauge(benchMetricPrefix+"last_request_latency_seconds", nil),

		configuredSeries:              set.NewGauge(benchMetricPrefix+"configured_series", nil),
		configuredConcurrency:         set.NewGauge(benchMetricPrefix+"configured_concurrency", nil),
		configuredMaxSeriesPerRequest: set.NewGauge(benchMetricPrefix+"configured_max_series_per_request", nil),
		configuredRequestInterval:     set.NewGauge(benchMetricPrefix+"configured_request_interval_seconds", nil),
		configuredSampleInterval:      set.NewGauge(benchMetricPrefix+"configured_sample_interval_seconds", nil),
		configuredUTF8Label:           set.NewGauge(benchMetricPrefix+"configured_utf8_label", nil),

		requestsTotal:                   set.NewCounter(benchMetricPrefix + "requests_total"),
		requestFailuresTotal:            set.NewCounter(benchMetricPrefix + "request_failures_total"),
		samplesSentTotal:                set.NewCounter(benchMetricPrefix + "samples_sent_total"),
		seriesSentTotal:                 set.NewCounter(benchMetricPrefix + "series_sent_total"),
		payloadBytesTotal:               set.NewCounter(benchMetricPrefix + "payload_bytes_total"),
		requestDurationSecondsHistogram: set.NewPrometheusHistogram(benchMetricPrefix + "request_duration_seconds"),
	}

	m.configuredSeries.Set(float64(cfg.Series))
	m.configuredConcurrency.Set(float64(cfg.Concurrency))
	m.configuredMaxSeriesPerRequest.Set(float64(cfg.MaxSeriesPerRequest))
	m.configuredRequestInterval.Set(cfg.RequestInterval.Seconds())
	m.configuredSampleInterval.Set(cfg.SampleInterval.Seconds())
	if cfg.UTF8Label {
		m.configuredUTF8Label.Set(1)
	}

	return m
}

func (m *benchMetrics) metricSet() *vmmetrics.Set {
	if m == nil {
		return nil
	}

	return m.set
}

func (m *benchMetrics) runStarted() {
	if m == nil {
		return
	}

	m.running.Set(1)
}

func (m *benchMetrics) runCompleted() {
	if m == nil {
		return
	}

	m.running.Set(0)
	m.inflightRequests.Set(0)
}

func (m *benchMetrics) requestStarted() {
	if m == nil {
		return
	}

	m.inflightRequests.Inc()
}

func (m *benchMetrics) requestFinished() {
	if m == nil {
		return
	}

	m.inflightRequests.Dec()
}

func (m *benchMetrics) recordSuccess(latency time.Duration, seriesCount int, sampleCount int, payloadBytes int) {
	if m == nil {
		return
	}

	m.requestsTotal.Inc()
	m.seriesSentTotal.Add(seriesCount)
	m.samplesSentTotal.Add(sampleCount)
	m.payloadBytesTotal.Add(payloadBytes)
	m.lastRequestLatencySeconds.Set(latency.Seconds())
	m.requestDurationSecondsHistogram.Update(latency.Seconds())
}

func (m *benchMetrics) recordFailure(latency time.Duration, payloadBytes int) {
	if m == nil {
		return
	}

	m.requestsTotal.Inc()
	m.requestFailuresTotal.Inc()
	m.payloadBytesTotal.Add(payloadBytes)
	m.lastRequestLatencySeconds.Set(latency.Seconds())
	m.requestDurationSecondsHistogram.Update(latency.Seconds())
}
