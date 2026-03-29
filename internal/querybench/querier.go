package querybench

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ClarkElll/my-tools/internal/schedutil"
)

type querier struct {
	cfg    Config
	client *http.Client
	logger *slog.Logger
	stats  *statsRecorder
}

func (q *querier) run(ctx context.Context, _ schedutil.Job) {
	var (
		reqURL string
		err    error
	)

	switch q.cfg.Endpoint {
	case EndpointQuery:
		reqURL, err = q.buildQueryURL()
	case EndpointQueryRange:
		reqURL, err = q.buildQueryRangeURL()
	case EndpointSeries:
		reqURL, err = q.buildSeriesURL()
	}

	if err != nil {
		q.logger.Error("query request build failed", "endpoint", q.cfg.Endpoint, "err", err)
		q.stats.recordFailure(0, "")
		return
	}

	q.execute(ctx, reqURL)
}

func (q *querier) execute(ctx context.Context, reqURL string) {
	reqCtx, cancel := context.WithTimeout(ctx, q.cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, reqURL, nil)
	if err != nil {
		q.stats.recordFailure(0, "")
		q.logger.Error("query request failed",
			"endpoint", q.cfg.Endpoint,
			"status", 0,
			"err", err,
		)
		return
	}

	start := time.Now()
	resp, err := q.client.Do(req)
	latency := time.Since(start)

	if err != nil {
		if ctx.Err() != nil {
			return
		}
		q.stats.recordFailure(latency, "")
		q.logger.Error("query request failed",
			"endpoint", q.cfg.Endpoint,
			"status", 0,
			"latency", latency,
			"err", err,
		)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		q.stats.recordFailure(latency, "")
		q.logger.Error("query request failed",
			"endpoint", q.cfg.Endpoint,
			"status", resp.StatusCode,
			"latency", latency,
			"body", strings.TrimSpace(string(body)),
		)
		return
	}

	body, _ := io.ReadAll(resp.Body)
	q.stats.recordSuccess(latency)
	q.logger.Info("query request succeeded",
		"endpoint", q.cfg.Endpoint,
		"status", resp.StatusCode,
		"latency", latency,
		"body_size", len(body),
	)
}

func (q *querier) buildQueryURL() (string, error) {
	u, err := url.Parse(q.cfg.URL + "/api/v1/query")
	if err != nil {
		return "", err
	}
	params := url.Values{}
	params.Set("query", q.cfg.Query)
	params.Set("time", formatUnixFloat(time.Now()))
	u.RawQuery = params.Encode()
	return u.String(), nil
}

func (q *querier) buildQueryRangeURL() (string, error) {
	u, err := url.Parse(q.cfg.URL + "/api/v1/query_range")
	if err != nil {
		return "", err
	}
	now := time.Now()
	end := now.Add(-q.cfg.Offset)
	start := end.Add(-q.cfg.Window)
	params := url.Values{}
	params.Set("query", q.cfg.Query)
	params.Set("start", formatUnixFloat(start))
	params.Set("end", formatUnixFloat(end))
	params.Set("step", fmt.Sprintf("%ds", int(q.cfg.Step.Seconds())))
	u.RawQuery = params.Encode()
	return u.String(), nil
}

func (q *querier) buildSeriesURL() (string, error) {
	u, err := url.Parse(q.cfg.URL + "/api/v1/series")
	if err != nil {
		return "", err
	}
	now := time.Now()
	end := now.Add(-q.cfg.Offset)
	start := end.Add(-q.cfg.Window)
	params := url.Values{}
	for _, m := range q.cfg.Match {
		params.Add("match[]", m)
	}
	params.Set("start", formatUnixFloat(start))
	params.Set("end", formatUnixFloat(end))
	u.RawQuery = params.Encode()
	return u.String(), nil
}

func formatUnixFloat(t time.Time) string {
	return strconv.FormatFloat(float64(t.UnixMilli())/1000.0, 'f', 3, 64)
}
