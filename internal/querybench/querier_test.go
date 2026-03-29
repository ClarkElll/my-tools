package querybench

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConfigNormalized(t *testing.T) {
	base := Config{
		URL:         "http://localhost:9090",
		Query:       "up",
		Endpoint:    EndpointQuery,
		QPS:         1,
		Concurrency: 1,
		Timeout:     10 * time.Second,
	}

	cfg, err := base.Normalized()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.URL != "http://localhost:9090" {
		t.Fatalf("unexpected url: %s", cfg.URL)
	}
}

func TestConfigNormalizedRejectsEmptyURL(t *testing.T) {
	_, err := Config{
		Endpoint: EndpointQuery, QPS: 1, Concurrency: 1, Timeout: time.Second,
	}.Normalized()
	if err == nil || !strings.Contains(err.Error(), "url is required") {
		t.Fatalf("expected url error, got %v", err)
	}
}

func TestConfigNormalizedRejectsInvalidEndpoint(t *testing.T) {
	_, err := Config{
		URL: "http://localhost:9090", Endpoint: "invalid", QPS: 1, Concurrency: 1, Timeout: time.Second,
	}.Normalized()
	if err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("expected endpoint error, got %v", err)
	}
}

func TestConfigNormalizedSeriesDefaultsMatchToQuery(t *testing.T) {
	cfg, err := Config{
		URL:         "http://localhost:9090",
		Query:       "up",
		Endpoint:    EndpointSeries,
		QPS:         1,
		Concurrency: 1,
		Timeout:     time.Second,
		Window:      time.Hour,
	}.Normalized()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Match) != 1 || cfg.Match[0] != "up" {
		t.Fatalf("expected match to default to query, got %v", cfg.Match)
	}
}

func TestConfigInterval(t *testing.T) {
	cfg := Config{QPS: 2}
	if cfg.Interval() != 500*time.Millisecond {
		t.Fatalf("expected 500ms, got %v", cfg.Interval())
	}
}

func testServer(t *testing.T, statusCode int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		io.WriteString(w, body)
	}))
}

func TestQuerierInstantQuery(t *testing.T) {
	srv := testServer(t, 200, `{"status":"success"}`)
	defer srv.Close()

	stats := &statsRecorder{}
	q := &querier{
		cfg: Config{
			URL:      srv.URL,
			Query:    "up",
			Endpoint: EndpointQuery,
			Timeout:  5 * time.Second,
		},
		client: srv.Client(),
		logger: nopLogger(),
		stats:  stats,
	}

	q.execute(context.Background(), srv.URL+"/api/v1/query?query=up&time=1234")

	if stats.requests.Load() != 1 || stats.failedRequests.Load() != 0 {
		t.Fatalf("expected 1 success, got requests=%d failed=%d",
			stats.requests.Load(), stats.failedRequests.Load())
	}
}

func TestQuerierFailsOnNon2xx(t *testing.T) {
	srv := testServer(t, 500, "internal server error")
	defer srv.Close()

	stats := &statsRecorder{}
	q := &querier{
		cfg:    Config{URL: srv.URL, Endpoint: EndpointQuery, Timeout: 5 * time.Second},
		client: srv.Client(),
		logger: nopLogger(),
		stats:  stats,
	}

	q.execute(context.Background(), srv.URL+"/api/v1/query")

	if stats.failedRequests.Load() != 1 {
		t.Fatalf("expected 1 failure, got %d", stats.failedRequests.Load())
	}
}

func TestBuildQueryRangeURL(t *testing.T) {
	q := &querier{
		cfg: Config{
			URL:      "http://localhost:9090",
			Query:    "rate(up[5m])",
			Endpoint: EndpointQueryRange,
			Offset:   5 * time.Minute,
			Window:   time.Hour,
			Step:     60 * time.Second,
		},
	}
	u, err := q.buildQueryRangeURL()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(u, "query_range") {
		t.Fatalf("expected query_range in url, got %s", u)
	}
	if !strings.Contains(u, "start=") || !strings.Contains(u, "end=") || !strings.Contains(u, "step=") {
		t.Fatalf("missing required params in url: %s", u)
	}
}

func TestBuildSeriesURL(t *testing.T) {
	q := &querier{
		cfg: Config{
			URL:    "http://localhost:9090",
			Match:  []string{"up", "go_info"},
			Offset: 5 * time.Minute,
			Window: time.Hour,
		},
	}
	u, err := q.buildSeriesURL()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(u, "series") {
		t.Fatalf("expected series in url, got %s", u)
	}
	if !strings.Contains(u, "match%5B%5D") && !strings.Contains(u, "match[]") {
		t.Fatalf("expected match[] in url, got %s", u)
	}
}

func nopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
