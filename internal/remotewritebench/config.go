package remotewritebench

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultMetricName = "vm_bench_series"
	UTF8LabelName     = "中文label"
	UTF8LabelValue    = "value"
)

type Config struct {
	URL                 string
	Series              int
	RequestInterval     time.Duration
	SampleInterval      time.Duration
	Concurrency         int
	MaxSeriesPerRequest int
	Duration            time.Duration
	Timeout             time.Duration
	UTF8Label           bool
	StartTime           time.Time
}

func (c Config) Normalized() (Config, error) {
	cfg := c
	cfg.URL = strings.TrimSpace(cfg.URL)
	if cfg.URL == "" {
		return cfg, fmt.Errorf("url is required")
	}

	parsedURL, err := url.Parse(cfg.URL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		if err == nil {
			err = fmt.Errorf("missing scheme or host")
		}
		return cfg, fmt.Errorf("invalid url %q: %w", cfg.URL, err)
	}

	if cfg.Series <= 0 {
		return cfg, fmt.Errorf("series must be greater than 0")
	}
	if cfg.RequestInterval <= 0 {
		return cfg, fmt.Errorf("request-interval must be greater than 0")
	}
	if cfg.SampleInterval <= 0 {
		return cfg, fmt.Errorf("sample-interval must be greater than 0")
	}
	if cfg.SampleInterval < time.Millisecond {
		return cfg, fmt.Errorf("sample-interval must be at least 1ms")
	}
	if cfg.Concurrency <= 0 {
		return cfg, fmt.Errorf("concurrency must be greater than 0")
	}
	if cfg.Duration < 0 {
		return cfg, fmt.Errorf("duration must be zero or positive")
	}
	if cfg.Timeout <= 0 {
		return cfg, fmt.Errorf("timeout must be greater than 0")
	}

	if cfg.StartTime.IsZero() {
		cfg.StartTime = time.Now()
	}

	if cfg.MaxSeriesPerRequest <= 0 || cfg.MaxSeriesPerRequest > cfg.Series {
		cfg.MaxSeriesPerRequest = cfg.Series
	}

	return cfg, nil
}
