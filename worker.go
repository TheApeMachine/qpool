package qpool

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Worker processes jobs
type Worker struct {
	pool   *Q
	jobs   chan Job
	cancel context.CancelFunc
}

func (w *Worker) start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			select {
			case w.pool.workers <- w.jobs:
				select {
				case job := <-w.jobs:
					// Add timeout for job processing
					done := make(chan struct{})
					go func() {
						result, err := w.processJob(job)
						w.pool.space.Store(job.ID, result, err, job.TTL)
						close(done)
					}()

					select {
					case <-done:
					case <-ctx.Done():
						return
					case <-time.After(30 * time.Second): // Timeout for job processing
						log.Printf("Job %s timed out", job.ID)
						continue
					}
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
					continue
				}
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
	}
}

func (w *Worker) processJob(job Job) (any, error) {
	startTime := job.StartTime

	// Circuit breaker check
	if err := w.checkCircuitBreaker(job.CircuitID); err != nil {
		return nil, err
	}

	// Dependency check
	if err := w.checkDependencies(job); err != nil {
		return nil, err
	}

	result, err := w.executeWithRetries(job)

	// Record metrics before storing result
	w.pool.metrics.recordJobExecution(startTime, err == nil)

	if err != nil {
		return nil, err
	}

	w.recordSuccess(job.CircuitID, startTime)
	return result, nil
}

func (w *Worker) executeWithRetries(job Job) (any, error) {
	for job.Attempt = 0; job.Attempt < job.RetryPolicy.MaxAttempts; job.Attempt++ {
		if job.Attempt > 0 {
			delay := job.RetryPolicy.Strategy.NextDelay(job.Attempt)
			log.Printf("Job %s retrying attempt %d after %v", job.ID, job.Attempt+1, delay)
			time.Sleep(delay)
		}

		result, err := job.Fn()
		if err == nil {
			return result, nil
		}

		job.LastError = err
		log.Printf("Job %s attempt %d failed with error: %v", job.ID, job.Attempt+1, err)
		w.recordFailure(job.CircuitID)

		if job.RetryPolicy.Filter != nil && !job.RetryPolicy.Filter(err) {
			break
		}
	}
	return nil, fmt.Errorf("all retries failed for job %s: %w", job.ID, job.LastError)
}

func (w *Worker) checkCircuitBreaker(circuitID string) error {
	if breaker := w.pool.breakers[circuitID]; breaker != nil {
		if !breaker.Allow() {
			err := fmt.Errorf("circuit breaker open for %s", circuitID)
			log.Printf("Job not allowed by circuit breaker %s", circuitID)
			return err
		}
	}
	return nil
}

func (w *Worker) recordSuccess(circuitID string, startTime time.Time) {
	if breaker := w.pool.breakers[circuitID]; breaker != nil {
		breaker.RecordSuccess()
	}
	duration := time.Since(startTime)
	w.pool.metrics.mu.Lock()
	w.pool.metrics.TotalJobTime += duration
	w.pool.metrics.JobCount++
	w.pool.metrics.mu.Unlock()
}

func (w *Worker) recordFailure(circuitID string) {
	if breaker := w.pool.breakers[circuitID]; breaker != nil {
		breaker.RecordFailure()
	}
}

func (w *Worker) checkDependencies(job Job) error {
	if len(job.Dependencies) == 0 {
		return nil
	}

	for _, depID := range job.Dependencies {
		if err := w.checkSingleDependency(depID, job.DependencyRetryPolicy); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) checkSingleDependency(depID string, retryPolicy *RetryPolicy) error {
	maxAttempts := 1
	var strategy RetryStrategy = &ExponentialBackoff{Initial: time.Second}

	if retryPolicy != nil {
		maxAttempts = retryPolicy.MaxAttempts
		strategy = retryPolicy.Strategy
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		ch := w.pool.space.Await(depID)
		if result := <-ch; result.Error == nil {
			return nil
		} else if attempt < maxAttempts-1 {
			time.Sleep(strategy.NextDelay(attempt + 1))
			continue
		} else {
			return fmt.Errorf("dependency %s failed: %w", depID, result.Error)
		}
	}
	return nil
}
