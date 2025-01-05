package qpool

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
)

// Worker processes jobs
type Worker struct {
	pool       *Q
	jobs       chan Job
	cancel     context.CancelFunc
	currentJob *Job // Added Field to Track Current Job
}

// run starts the worker's job processing loop
func (w *Worker) run() {
	jobChan := w.jobs // Store the job channel locally for clarity

	for {
		// First check if we should exit
		select {
		case <-w.pool.ctx.Done():
			errnie.Info("Worker exiting due to context cancellation")
			return
		default:
		}

		// Register ourselves as available
		errnie.Info("Worker registering as available")
		w.pool.workers <- jobChan

		// Wait for a job
		select {
		case <-w.pool.ctx.Done():
			errnie.Info("Worker exiting while waiting for job")
			return
		case job, ok := <-jobChan:
			if !ok {
				errnie.Warn("Worker job channel closed")
				return
			}

			errnie.Info("Worker received job: %s", job.ID)
			w.currentJob = &job
			result, err := w.processJobWithTimeout(w.pool.ctx, job)
			w.currentJob = nil
			errnie.Info("Worker completed job: %s, err: %v", job.ID, err)

			// Handle result
			if err != nil {
				w.pool.metrics.RecordJobFailure()
				errnie.Error(fmt.Errorf("Job %s failed: %v", job.ID, err))
				// Store error result
				w.pool.space.StoreError(job.ID, err, job.TTL)
			} else {
				w.pool.metrics.RecordJobSuccess(time.Since(job.StartTime))
				errnie.Info("Job %s succeeded", job.ID)
				// Store successful result
				w.pool.space.Store(job.ID, result, []State{{Value: result, Probability: 1.0}}, job.TTL)
			}
			errnie.Info("Stored result for job: %s", job.ID)

			// Notify dependents
			if len(job.Dependencies) > 0 {
				for _, depID := range job.Dependencies {
					if children := w.pool.space.children[depID]; len(children) > 0 {
						for _, childID := range children {
							errnie.Info("Notifying dependent job %s", childID)
						}
					}
				}
			}
		}
	}
}

// processJobWithTimeout processes a job with a timeout
func (w *Worker) processJobWithTimeout(ctx context.Context, job Job) (any, error) {
	startTime := time.Now()

	// Check dependencies before job execution
	for _, depID := range job.Dependencies {
		if err := w.checkSingleDependency(depID, job.DependencyRetryPolicy); err != nil {
			w.pool.metrics.RecordJobExecution(startTime, false)
			if job.CircuitID != "" {
				w.recordFailure(job.CircuitID)
			}
			return nil, err
		}
	}

	done := make(chan struct{})
	var result any
	var err error

	go func() {
		defer close(done)
		result, err = job.Fn()
		errnie.Info("Job %s completed", job.ID)
	}()

	select {
	case <-ctx.Done():
		w.pool.metrics.RecordJobFailure()
		return nil, errnie.Error(fmt.Errorf("job %s timed out", job.ID))
	case <-done:
		w.pool.metrics.RecordJobExecution(startTime, err == nil)
		return result, err
	}
}

// checkSingleDependency checks a single job dependency with retries
func (w *Worker) checkSingleDependency(depID string, retryPolicy *RetryPolicy) error {
	maxAttempts := 1
	var strategy RetryStrategy = &ExponentialBackoff{Initial: time.Second}

	if retryPolicy != nil {
		maxAttempts = retryPolicy.MaxAttempts
		strategy = retryPolicy.Strategy
	}

	circuitID := ""
	if w.currentJob != nil {
		circuitID = w.currentJob.CircuitID
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// First check if the dependency exists
		if !w.pool.space.Exists(depID) {
			if attempt < maxAttempts-1 {
				time.Sleep(strategy.NextDelay(attempt + 1))
				continue
			}
			break
		}

		ch := w.pool.space.Await(depID)
		select {
		case result := <-ch:
			if result.Error == nil {
				return nil
			}
		case <-time.After(time.Second): // Add timeout for each attempt
			// Continue to next attempt or break
		}

		if attempt < maxAttempts-1 {
			time.Sleep(strategy.NextDelay(attempt + 1))
			continue
		}
	}

	w.pool.breakersMu.RLock()
	breaker, exists := w.pool.breakers[circuitID]
	w.pool.breakersMu.RUnlock()

	if exists {
		breaker.RecordFailure()
	}

	w.pool.space.mu.Lock()
	if w.pool.space.children == nil {
		w.pool.space.children = make(map[string][]string)
	}
	if w.currentJob != nil {
		w.pool.space.children[depID] = append(w.pool.space.children[depID], w.currentJob.ID)
	}
	w.pool.space.mu.Unlock()

	return errnie.Error(fmt.Errorf("dependency %s failed after %d attempts", depID, maxAttempts))
}

// recordFailure records a failure for a specific circuit breaker
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

// Add this method to the Worker struct
func (w *Worker) handleJobTimeout(job Job) error {
	w.pool.metrics.RecordJobFailure()
	return errnie.Error(fmt.Errorf("job %s timed out", job.ID))
}
