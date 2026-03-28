package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ClarkElll/my-tools/internal/cliutil"
	"github.com/ClarkElll/my-tools/internal/logutil"
	"github.com/ClarkElll/my-tools/internal/remotewritebench"
)

func main() {
	fs := flag.NewFlagSet("remote-write-bench", flag.ExitOnError)

	var (
		url                 = fs.String("url", "", "Prometheus remote write endpoint, for example http://localhost:9090/api/v1/write")
		series              = fs.Int("series", 1000, "total number of distinct time series to generate")
		sampleInterval      = fs.Duration("sample-interval", 15*time.Second, "timestamp step between generated points on the same series; if smaller than request-interval, multiple samples may be batched into one write request")
		requestInterval     = fs.Duration("request-interval", time.Second, "actual interval between scheduled remote write sends")
		concurrency         = fs.Int("concurrency", 1, "number of workers used to keep scheduled sends on time when earlier requests are still in flight")
		maxSeriesPerRequest = fs.Int("max-series-per-request", 0, "maximum number of series carried by a single write request; 0 sends all series in one request")
		timeout             = fs.Duration("timeout", 10*time.Second, "per-request timeout")
		duration            = fs.Duration("duration", 0, "total benchmark duration; 0 runs until interrupted")
		utf8Label           = fs.Bool("utf8-label", false, "append a fixed UTF-8 label 中文label=value to every generated series")
		realistic           = fs.Bool("realistic", false, "enable realistic mode: generate series with Kubernetes-like labels (job, namespace, pod, container, instance) and simulate pod churn")
	)

	fs.Usage = cliutil.NewUsage(fs, cliutil.Tool{
		Name:        "remote-write-bench",
		Description: "Generate fixed series sets and push pending samples for each series to a Prometheus remote write endpoint on every scheduled send.",
		Invocation:  "go run ./app/remote-write-bench",
	})

	fs.Parse(os.Args[1:])

	logger := logutil.New(os.Stderr, logutil.Options{}).With("tool", "remote-write-bench")

	cfg := remotewritebench.Config{
		URL:                 *url,
		Series:              *series,
		RequestInterval:     *requestInterval,
		SampleInterval:      *sampleInterval,
		Concurrency:         *concurrency,
		MaxSeriesPerRequest: *maxSeriesPerRequest,
		Duration:            *duration,
		Timeout:             *timeout,
		UTF8Label:           *utf8Label,
		Realistic:           *realistic,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := remotewritebench.Run(ctx, logger, cfg); err != nil {
		exitWithError(logger, err)
	}
}

func exitWithError(logger *slog.Logger, err error) {
	if logger != nil {
		logger.Error("remote-write-bench failed", "err", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
