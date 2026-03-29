package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ClarkElll/my-tools/internal/cliutil"
	"github.com/ClarkElll/my-tools/internal/logutil"
	"github.com/ClarkElll/my-tools/internal/querybench"
)

func main() {
	fs := flag.NewFlagSet("query-bench", flag.ExitOnError)

	var (
		url         = fs.String("url", "", "Prometheus base URL, for example http://localhost:9090")
		query       = fs.String("query", "up", "PromQL expression (used by query and query_range endpoints)")
		endpoint    = fs.String("endpoint", "query", "API endpoint to benchmark: query, query_range, or series")
		qps         = fs.Float64("qps", 1, "target queries per second (e.g. 0.5 sends one request every 2 seconds)")
		concurrency = fs.Int("concurrency", 1, "number of concurrent workers")
		timeout     = fs.Duration("timeout", 10*time.Second, "per-request timeout")
		duration    = fs.Duration("duration", 0, "total benchmark duration; 0 runs until interrupted")
		step        = fs.Duration("step", 60*time.Second, "step for query_range requests")
		offset      = fs.Duration("offset", 5*time.Minute, "end = now()-offset for query_range and series")
		window      = fs.Duration("window", time.Hour, "time window: start = now()-offset-window for query_range and series")
		match       = fs.String("match", "", "match[] selector for series endpoint (comma-separated); defaults to -query value")
	)

	fs.Usage = cliutil.NewUsage(fs, cliutil.Tool{
		Name:        "query-bench",
		Description: "Benchmark Prometheus HTTP API query endpoints (query, query_range, series) at a target QPS.",
		Invocation:  "go run ./app/query-bench",
	})

	fs.Parse(os.Args[1:])

	logger := logutil.New(os.Stderr, logutil.Options{}).With("tool", "query-bench")

	var matchSlice []string
	if *match != "" {
		for _, m := range strings.Split(*match, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				matchSlice = append(matchSlice, m)
			}
		}
	}

	cfg := querybench.Config{
		URL:         *url,
		Query:       *query,
		Endpoint:    *endpoint,
		QPS:         *qps,
		Concurrency: *concurrency,
		Timeout:     *timeout,
		Duration:    *duration,
		Step:        *step,
		Offset:      *offset,
		Window:      *window,
		Match:       matchSlice,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := querybench.Run(ctx, logger, cfg); err != nil {
		exitWithError(logger, err)
	}
}

func exitWithError(logger *slog.Logger, err error) {
	if logger != nil {
		logger.Error("query-bench failed", "err", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
