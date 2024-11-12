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
					// Create timeout context for job processing
					jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
					done := make(chan struct{})

					go func() {
						defer close(done)

						// Use jobCtx to handle cancellation
						result, err := w.processJobWithTimeout(jobCtx, job)

						// Only store result if we haven't timed out
						select {
						case <-jobCtx.Done():
							if jobCtx.Err() == context.DeadlineExceeded {
								w.handleJobTimeout(job)
							}
							return
						default:
							if err != nil {
								w.handleJobError(job, err)
							}
							w.pool.space.Store(job.ID, result, err, job.TTL)
						}
					}()

					// Wait for completion or timeout
					select {
					case <-done:
						cancel() // Clean up context
					case <-jobCtx.Done():
						cancel() // Clean up context
						if jobCtx.Err() == context.DeadlineExceeded {
							log.Printf("Job %s timed out after %v", job.ID, 30*time.Second)
						}
					}
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
	}
}

func (w *Worker) processJobWithTimeout(ctx context.Context, job Job) (any, error) {
	resultCh := make(chan any, 1)
	errCh := make(chan error, 1)

	go func() {
		result, err := w.processJob(job)
		select {
		case <-ctx.Done():
			return
		default:
			resultCh <- result
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		return result, <-errCh
	}
}

func (w *Worker) handleJobTimeout(job Job) {
	timeoutErr := fmt.Errorf("job processing timed out after %v", 30*time.Second)

	// Log timeout with context
	log.Printf("Job timeout - ID: %s, Attempt: %d/%d, CircuitID: %s, Duration: %v",
		job.ID,
		job.Attempt+1,
		job.RetryPolicy.MaxAttempts,
		job.CircuitID,
		time.Since(job.StartTime),
	)

	// Update metrics
	w.pool.metrics.mu.Lock()
	w.pool.metrics.ErrorRates[job.CircuitID]++
	w.pool.metrics.mu.Unlock()

	// Store timeout error
	w.pool.space.Store(job.ID, nil, timeoutErr, job.TTL)

	// Record failure for circuit breaker if configured
	if job.CircuitID != "" {
		w.recordFailure(job.CircuitID)
	}
}

func (w *Worker) handleJobError(job Job, err error) {
	// Log error with context
	log.Printf("Job processing failed - ID: %s, Attempt: %d/%d, Error: %v, CircuitID: %s, Duration: %v",
		job.ID,
		job.Attempt+1,
		job.RetryPolicy.MaxAttempts,
		err,
		job.CircuitID,
		time.Since(job.StartTime),
	)

	// Update metrics
	w.pool.metrics.mu.Lock()
	w.pool.metrics.ErrorRates[job.CircuitID]++
	w.pool.metrics.mu.Unlock()

	// Record failure for circuit breaker if configured
	if job.CircuitID != "" {
		w.recordFailure(job.CircuitID)
	}
}

func (w *Worker) processJob(job Job) (any, error) {
	startTime := time.Now()

	// Get or create circuit breaker
	breaker := w.pool.getCircuitBreaker(job)
	if breaker != nil {
		if !breaker.Allow() {
			return nil, fmt.Errorf("circuit breaker %s is open", job.CircuitID)
		}
	}

	// Check dependencies before processing
	if err := w.checkDependencies(job); err != nil {
		if breaker != nil {
			breaker.RecordFailure()
		}
		return nil, fmt.Errorf("dependency check failed: %w", err)
	}

	// Process the job with retry if configured
	result, err := w.executeWithRetry(job)

	// Record metrics for job execution
	success := err == nil
	w.pool.metrics.recordJobExecution(startTime, success)

	// Record success/failure for circuit breaker
	if breaker != nil {
		if err != nil {
			breaker.RecordFailure()
		} else {
			breaker.RecordSuccess()
		}
	}

	return result, err
}

func (w *Worker) executeWithRetry(job Job) (any, error) {
	var result any
	var err error

	for attempt := 0; attempt <= job.RetryPolicy.MaxAttempts; attempt++ {
		result, err = job.Fn()
		if err == nil {
			return result, nil
		}

		// Record metrics for failed attempt
		if attempt < job.RetryPolicy.MaxAttempts {
			w.pool.metrics.recordJobExecution(time.Now(), false)
		}

		// Wait before retry if not the last attempt
		if attempt < job.RetryPolicy.MaxAttempts {
			time.Sleep(job.RetryPolicy.Strategy.NextDelay(attempt))
		}
	}

	return result, err
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

func (q *Q) Close() {
	q.breakersMu.Lock()
	defer q.breakersMu.Unlock()

	// Clear circuit breakers
	q.breakers = make(map[string]*CircuitBreaker)

	// Cancel all worker contexts
	q.workerMu.Lock()
	for _, worker := range q.workerList {
		if worker.cancel != nil {
			worker.cancel()
		}
	}
	q.workerList = nil
	q.workerMu.Unlock()

	// Close worker channels
	close(q.jobs)
	close(q.workers)

	// Close quantum space
	if q.space != nil {
		q.space.Close()
	}
}

// Add recordFailure method to Worker struct
func (w *Worker) recordFailure(circuitID string) {
	if circuitID == "" {
		return
	}

	w.pool.breakersMu.RLock()
	breaker, exists := w.pool.breakers[circuitID]
	w.pool.breakersMu.RUnlock()

	if exists {
		breaker.RecordFailure()
	}
}
