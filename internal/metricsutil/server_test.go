package metricsutil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	vmmetrics "github.com/VictoriaMetrics/metrics"
)

func TestConfigNormalizedDefaultsPath(t *testing.T) {
	cfg, err := Config{
		ListenAddr: " 127.0.0.1:9098 ",
	}.Normalized()
	if err != nil {
		t.Fatalf("Normalized returned error: %v", err)
	}

	if cfg.ListenAddr != "127.0.0.1:9098" {
		t.Fatalf("expected trimmed listen addr, got %q", cfg.ListenAddr)
	}
	if cfg.Path != DefaultPath {
		t.Fatalf("expected default path %q, got %q", DefaultPath, cfg.Path)
	}
}

func TestConfigNormalizedRejectsPathWithoutSlash(t *testing.T) {
	_, err := Config{Path: "metrics"}.Normalized()
	if err == nil || !strings.Contains(err.Error(), "must start with /") {
		t.Fatalf("expected invalid path error, got %v", err)
	}
}

func TestNewHandlerServesMetrics(t *testing.T) {
	set := vmmetrics.NewSet()
	counter := set.NewCounter("test_counter_total")
	counter.Add(2)

	handler, cfg, err := NewHandler(Config{Path: "/internal/metrics"}, set)
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, cfg.Path, nil)
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "# TYPE test_counter_total counter") {
		t.Fatalf("expected counter metadata, got %q", body)
	}
	if !strings.Contains(body, "test_counter_total 2") {
		t.Fatalf("expected counter value in response, got %q", body)
	}
}

func TestNewHandlerRejectsMethod(t *testing.T) {
	handler, cfg, err := NewHandler(Config{}, vmmetrics.NewSet())
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, cfg.Path, nil)
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", recorder.Code)
	}
	if got := recorder.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("expected Allow header %q, got %q", "GET, HEAD", got)
	}
}
