package qpool

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/phuslu/log"
)

func processJob(q *Q, workerCtx context.Context, job Job) {
	deadline := q.schedulingTimeout()

	if job.ExecTimeout > 0 {
		deadline = job.ExecTimeout
	}

	execCtx, cancel := context.WithTimeout(workerCtx, deadline)
	defer cancel()

	startedAt := time.Now()

	q.publishTelemetry(Event{
		Component: "qpool",
		Op:        "job-start",
		Message:   fmt.Sprintf("job started: %s", job.ID),
		Time:      startedAt,
		Level:     log.InfoLevel,
		Fields: []Field{
			{Key: "job", Value: job.ID},
		},
	})

	result, err := runJobWithRetries(execCtx, job)

	latency := time.Since(job.StartTime)
	execDur := time.Since(startedAt)

	if err != nil {
		q.metrics.RecordJobOutcome(latency, false)

		if job.CircuitID != "" {
			if cb := q.breakerForJob(&job); cb != nil {
				cb.RecordFailure()
			}
		}

		q.publishTelemetry(Event{
			Component: "qpool",
			Op:        "job-error",
			Message:   fmt.Sprintf("job failed: %s (%v)", job.ID, err),
			Time:      time.Now(),
			Level:     log.ErrorLevel,
			Err:       err,
			Fields: []Field{
				{Key: "job", Value: job.ID},
				{Key: "phase", Value: "execution"},
				{Key: "duration_ms", Value: latency.Milliseconds()},
				{Key: "exec_duration_ms", Value: execDur.Milliseconds()},
			},
		})

		q.space.StoreError(job.ID, err, job.TTL)

		return
	}

	q.metrics.RecordJobOutcome(latency, true)

	if job.CircuitID != "" {
		if cb := q.breakerForJob(&job); cb != nil {
			cb.RecordSuccess()
		}
	}

	q.publishTelemetry(Event{
		Component: "qpool",
		Op:        "job-complete",
		Message:   fmt.Sprintf("job completed: %s in %s", job.ID, latency.Round(time.Millisecond)),
		Time:      time.Now(),
		Level:     log.InfoLevel,
		Fields: []Field{
			{Key: "job", Value: job.ID},
			{Key: "duration_ms", Value: latency.Milliseconds()},
			{Key: "exec_duration_ms", Value: execDur.Milliseconds()},
		},
	})

	q.space.Store(job.ID, result, job.TTL)
}

func runJobWithRetries(ctx context.Context, job Job) (any, error) {
	maxAttempts := 1

	strategy := RetryStrategy(&ExponentialBackoff{Initial: time.Second})

	if job.RetryPolicy != nil {
		if job.RetryPolicy.MaxAttempts > 0 {
			maxAttempts = job.RetryPolicy.MaxAttempts
		}

		if job.RetryPolicy.Strategy != nil {
			strategy = job.RetryPolicy.Strategy
		}
	}

	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		res, err := invokeFnOnce(ctx, job)

		if err == nil {
			return res, nil
		}

		lastErr = err

		if job.RetryPolicy != nil && job.RetryPolicy.Filter != nil && !job.RetryPolicy.Filter(err) {
			break
		}

		if attempt == maxAttempts {
			break
		}

		delay := strategy.NextDelay(attempt)

		if delay <= 0 {
			delay = time.Millisecond
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}

	return nil, lastErr
}

func invokeFnOnce(ctx context.Context, job Job) (res any, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil

			err = fmt.Errorf("qpool: panic in job %s: %v\n%s", job.ID, r, debug.Stack())
		}
	}()

	return job.Fn(ctx)
}
