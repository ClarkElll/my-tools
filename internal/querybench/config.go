package querybench

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	EndpointQuery      = "query"
	EndpointQueryRange = "query_range"
	EndpointSeries     = "series"
)

type Config struct {
	URL         string
	Query       string
	Endpoint    string
	QPS         float64
	Concurrency int
	Timeout     time.Duration
	Duration    time.Duration
	Step        time.Duration
	Offset      time.Duration
	Window      time.Duration
	Match       []string
	ListenAddr  string
}

func (c Config) Interval() time.Duration {
	return time.Duration(float64(time.Second) / c.QPS)
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

	switch cfg.Endpoint {
	case EndpointQuery, EndpointQueryRange, EndpointSeries:
	default:
		return cfg, fmt.Errorf("endpoint must be one of: query, query_range, series")
	}

	if cfg.QPS <= 0 {
		return cfg, fmt.Errorf("qps must be greater than 0")
	}
	if cfg.Concurrency <= 0 {
		return cfg, fmt.Errorf("concurrency must be greater than 0")
	}
	if cfg.Timeout <= 0 {
		return cfg, fmt.Errorf("timeout must be greater than 0")
	}
	if cfg.Duration < 0 {
		return cfg, fmt.Errorf("duration must be zero or positive")
	}

	if cfg.Endpoint == EndpointQueryRange {
		if cfg.Step <= 0 {
			return cfg, fmt.Errorf("step must be greater than 0 for query_range")
		}
		if cfg.Window <= 0 {
			return cfg, fmt.Errorf("window must be greater than 0 for query_range")
		}
	}

	if cfg.Endpoint == EndpointSeries {
		if cfg.Window <= 0 {
			return cfg, fmt.Errorf("window must be greater than 0 for series")
		}
		if len(cfg.Match) == 0 {
			cfg.Match = []string{cfg.Query}
		}
	}

	return cfg, nil
}
