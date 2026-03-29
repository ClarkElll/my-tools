package schedutil

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunRejectsInvalidOptions(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{
			name: "missing interval",
			opts: Options{
				Workers: 1,
				Run:     func(context.Context, Job) {},
			},
		},
		{
			name: "missing workers",
			opts: Options{
				Interval: time.Second,
				Run:      func(context.Context, Job) {},
			},
		},
		{
			name: "missing callback",
			opts: Options{
				Interval: time.Second,
				Workers:  1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Run(context.Background(), tt.opts); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestRunSchedulesImmediateJobAndTickerJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	jobs := make([]Job, 0, 3)

	err := Run(ctx, Options{
		Interval:  5 * time.Millisecond,
		Workers:   1,
		Immediate: true,
		Run: func(_ context.Context, job Job) {
			mu.Lock()
			jobs = append(jobs, job)
			count := len(jobs)
			mu.Unlock()

			if count == 3 {
				cancel()
			}
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}
	for index, job := range jobs {
		if got, want := job.Sequence, int64(index); got != want {
			t.Fatalf("job %d expected sequence %d, got %d", index, want, got)
		}
	}
}

func TestRunPreservesScheduledJobsWithBacklog(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var active atomic.Int64
	var maxActive atomic.Int64
	var completed atomic.Int64
	var mu sync.Mutex
	sequences := make([]int64, 0, 4)

	err := Run(ctx, Options{
		Interval:      5 * time.Millisecond,
		Workers:       2,
		QueueCapacity: 1,
		Immediate:     true,
		Run: func(_ context.Context, job Job) {
			currentActive := active.Add(1)
			for {
				observedMax := maxActive.Load()
				if currentActive <= observedMax {
					break
				}
				if maxActive.CompareAndSwap(observedMax, currentActive) {
					break
				}
			}

			time.Sleep(20 * time.Millisecond)

			mu.Lock()
			sequences = append(sequences, job.Sequence)
			mu.Unlock()

			active.Add(-1)
			if completed.Add(1) == 4 {
				cancel()
			}
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if got := completed.Load(); got < 4 {
		t.Fatalf("expected at least 4 completed jobs, got %d", got)
	}
	slices.Sort(sequences)
	if !slices.Equal(sequences[:4], []int64{0, 1, 2, 3}) {
		t.Fatalf("expected the first scheduled sequences to include [0 1 2 3], got %v", sequences)
	}
	if maxActive.Load() < 2 {
		t.Fatalf("expected overlapping execution across workers, got maxActive=%d", maxActive.Load())
	}
}
