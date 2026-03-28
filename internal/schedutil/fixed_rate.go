package schedutil

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Job struct {
	Sequence    int64
	ScheduledAt time.Time
}

type Options struct {
	Interval      time.Duration
	Workers       int
	QueueCapacity int
	Immediate     bool
	Run           func(context.Context, Job)
}

func Run(ctx context.Context, opts Options) error {
	if err := opts.validate(); err != nil {
		return err
	}

	queues := make([]chan Job, opts.Workers)
	var workers sync.WaitGroup
	for index := 0; index < opts.Workers; index++ {
		queue := make(chan Job, opts.queueCapacity())
		queues[index] = queue

		workers.Add(1)
		go func(workerQueue <-chan Job) {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-workerQueue:
					if !ok {
						return
					}
					if ctx.Err() != nil {
						return
					}

					opts.Run(ctx, job)
				}
			}
		}(queue)
	}

	defer func() {
		for _, queue := range queues {
			close(queue)
		}
		workers.Wait()
	}()

	dispatcher := roundRobinDispatcher{queues: queues}
	var sequence int64
	submitJob := func(scheduledAt time.Time) error {
		job := Job{
			Sequence:    sequence,
			ScheduledAt: scheduledAt,
		}
		sequence++
		return dispatcher.submit(ctx, job)
	}

	if opts.Immediate {
		if err := submitJob(time.Now()); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}

	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case tickAt := <-ticker.C:
			if err := submitJob(tickAt); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

func (o Options) validate() error {
	if o.Interval <= 0 {
		return fmt.Errorf("interval must be greater than 0")
	}
	if o.Workers <= 0 {
		return fmt.Errorf("workers must be greater than 0")
	}
	if o.Run == nil {
		return fmt.Errorf("run callback is required")
	}

	return nil
}

func (o Options) queueCapacity() int {
	if o.QueueCapacity <= 0 {
		return 1
	}

	return o.QueueCapacity
}

type roundRobinDispatcher struct {
	queues []chan Job
	next   int
}

func (d *roundRobinDispatcher) submit(ctx context.Context, job Job) error {
	if len(d.queues) == 0 {
		return nil
	}

	queue := d.queues[d.next]
	d.next = (d.next + 1) % len(d.queues)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case queue <- job:
		return nil
	}
}
