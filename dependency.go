package qpool

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/phuslu/log"
)

func (q *Q[T]) startDependencyWait(job Job) error {
	if q.stopping.Load() {
		return fmt.Errorf("qpool: pool closed")
	}

	if err := q.ctx.Err(); err != nil {
		return fmt.Errorf("qpool: pool closed: %w", err)
	}

	q.deps.Add(1)

	go q.resolveDependentJob(job)

	return nil
}

func (q *Q[T]) resolveDependentJob(job Job) {
	defer q.deps.Done()

	if err := q.waitDependencies(q.ctx, job); err != nil {
		q.recordDependencyFailure(job, err)

		return
	}

	enqueueCtx, cancel := context.WithTimeout(q.ctx, q.schedulingTimeout())
	defer cancel()

	if err := q.enqueueJob(enqueueCtx, job); err != nil {
		q.space.StoreError(job.ID, err, job.TTL)
	}
}

func (q *Q[T]) recordDependencyFailure(job Job, err error) {
	latency := time.Since(job.StartTime)
	q.metrics.RecordJobOutcome(latency, false)

	if job.CircuitID != "" {
		if breaker := q.breakerForJob(&job); breaker != nil {
			breaker.RecordFailure()
		}
	}

	q.publishTelemetry(Event{
		Component: "qpool",
		Op:        "job-error",
		Message:   fmt.Sprintf("dependencies unmet: %s (%v)", job.ID, err),
		Time:      time.Now(),
		Level:     log.WarnLevel,
		Err:       err,
		Fields: []Field{
			{Key: "job", Value: job.ID},
			{Key: "phase", Value: "dependency"},
			{Key: "duration_ms", Value: latency.Milliseconds()},
		},
	})

	q.space.StoreError(job.ID, err, job.TTL)
}

func (q *Q[T]) waitDependencies(dependencyCtx context.Context, job Job) error {
	if len(job.Dependencies) == 0 {
		return nil
	}

	var (
		waitGroup atomicWaitGroup
		firstErr  atomic.Pointer[error]
	)

	waitGroup.Add(int64(len(job.Dependencies)))

	for _, dependencyID := range job.Dependencies {
		dependencyID := dependencyID

		go func() {
			defer waitGroup.Done()

			if err := q.waitOneDependency(dependencyCtx, job, dependencyID); err != nil {
				for {
					if current := firstErr.Load(); current != nil {
						return
					}

					if firstErr.CompareAndSwap(nil, &err) {
						return
					}
				}
			}
		}()
	}

	waitGroup.Wait()

	if errPtr := firstErr.Load(); errPtr != nil {
		return *errPtr
	}

	return dependencyCtx.Err()
}

func dependencyAwaitTimeout(policy *RetryPolicy, strategy RetryStrategy) time.Duration {
	if policy != nil && policy.PerAttemptTimeout > 0 {
		return policy.PerAttemptTimeout
	}

	var base time.Duration

	if strategy != nil {
		base = strategy.NextDelay(1)
	}

	if base <= 0 {
		base = time.Second
	}

	const maxDerived = 60 * time.Second

	if base > maxDerived {
		return maxDerived
	}

	if base < time.Second {
		return time.Second
	}

	return base
}

func (q *Q[T]) waitOneDependency(
	dependencyCtx context.Context,
	job Job,
	dependencyID string,
) error {
	maxAttempts := 1
	strategy := RetryStrategy(&ExponentialBackoff{Initial: time.Second})
	var lastErr error

	if job.DependencyRetryPolicy != nil {
		maxAttempts = job.DependencyRetryPolicy.MaxAttempts

		if job.DependencyRetryPolicy.Strategy != nil {
			strategy = job.DependencyRetryPolicy.Strategy
		}
	}

	awaitTimeout := dependencyAwaitTimeout(job.DependencyRetryPolicy, strategy)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		wait := q.space.Await(dependencyID)
		waitCtx, cancel := context.WithTimeout(dependencyCtx, awaitTimeout)

		result, err := wait.Get(waitCtx)
		cancel()

		if err == nil {
			if result == nil {
				lastErr = fmt.Errorf("dependency %s returned nil result", dependencyID)

				if attempt < maxAttempts-1 {
					time.Sleep(strategy.NextDelay(attempt + 1))
				}

				continue
			}

			if result.Error != nil {
				return fmt.Errorf("dependency %s: %w", dependencyID, result.Error)
			}

			return nil
		}

		lastErr = err

		if err := dependencyCtx.Err(); err != nil {
			return fmt.Errorf("dependency %s: %w", dependencyID, err)
		}

		if attempt < maxAttempts-1 {
			time.Sleep(strategy.NextDelay(attempt + 1))
		}
	}

	q.space.RegisterDependent(dependencyID, job.ID)

	if lastErr != nil {
		return fmt.Errorf(
			"dependency %s failed after %d attempts: %w",
			dependencyID,
			maxAttempts,
			lastErr,
		)
	}

	return fmt.Errorf("dependency %s failed after %d attempts", dependencyID, maxAttempts)
}
