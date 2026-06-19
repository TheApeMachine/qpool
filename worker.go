package qpool

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/theapemachine/datura"
)

func processJob(q *Q[any], workerCtx context.Context, job Job) {
	deadline := q.schedulingTimeout()

	if job.ExecTimeout > 0 {
		deadline = job.ExecTimeout
	}

	execCtx, cancel := context.WithTimeout(workerCtx, deadline)
	defer cancel()

	startedAt := time.Now()

	startedEvent := datura.Acquire(
		"qpool",
		datura.Artifact_TypeFromString("debug"),
	)
	payload, err := json.Marshal(map[string]any{
		"job": job.ID,
	})

	if err != nil {
		er, err := datura.NewArtifact_Error(startedEvent.Segment())
		if err != nil {
			return
		}
		er.SetType(datura.Artifact_Error_Type(datura.Artifact_Type_json))
		er.SetTimestamp(time.Now().Unix())
		startedEvent.SetError(er)
	}

	startedEvent.WithPayload(payload)
	startedEvent.SetTimestamp(startedAt.Unix())
	q.publishTelemetry(startedEvent)

	result, err := runJobWithRetries(execCtx, job)

	latency := time.Since(job.StartTime)
	execDur := time.Since(startedAt)

	if err != nil {
		q.metrics.RecordJobOutcome(latency, false)

		if job.CircuitID != "" {
			if cb := q.breakerForJob(job); cb != nil {
				cb.RecordFailure()
			}
		}

		artifact := datura.Acquire(
			"qpool",
			datura.Artifact_TypeFromString("error"),
		)

		er, err := datura.NewArtifact_Error(artifact.Segment())
		artifact.SetError(er)
		artifact.SetTimestamp(time.Now().Unix())

		q.publishTelemetry(artifact)

		q.space.StoreError(job.ID, err, job.TTL)

		return
	}

	q.metrics.RecordJobOutcome(latency, true)

	if job.CircuitID != "" {
		if cb := q.breakerForJob(job); cb != nil {
			cb.RecordSuccess()
		}
	}

	completeEvent := datura.Acquire(
		"qpool",
		datura.Artifact_TypeFromString("debug"),
	)
	payload, err = json.Marshal(map[string]any{
		"job":              job.ID,
		"duration_ms":      latency.Milliseconds(),
		"exec_duration_ms": execDur.Milliseconds(),
	})

	if err != nil {
		return
	}
	completeEvent.WithPayload(payload)
	completeEvent.SetTimestamp(time.Now().Unix())
	q.publishTelemetry(completeEvent)

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
