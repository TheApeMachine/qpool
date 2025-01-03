package qpool

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

/*
	Q is our hybrid worker pool/message queue implementation.

It combines traditional worker pool functionality with quantum-inspired state management.
The pool maintains a balance between worker availability and job scheduling while
providing quantum-like properties such as state superposition and entanglement through
its integration with QSpace.

Key features:
  - Dynamic worker scaling
  - Circuit breaker pattern support
  - Quantum-inspired state management
  - Metrics collection and monitoring
*/
type Q struct {
	ctx        context.Context
	cancel     context.CancelFunc
	quit       chan struct{}
	wg         sync.WaitGroup
	workers    chan chan Job
	jobs       chan Job
	space      *QSpace
	scaler     *Scaler
	metrics    *Metrics
	breakers   map[string]*CircuitBreaker
	workerMu   sync.Mutex
	workerList []*Worker
	breakersMu sync.RWMutex
	config     *Config
}

/*
	NewQ creates a new quantum pool with the specified worker constraints and configuration.

The pool initializes with the minimum number of workers and scales dynamically based on load.

Parameters:
  - ctx: Parent context for lifecycle management
  - minWorkers: Minimum number of workers to maintain
  - maxWorkers: Maximum number of workers allowed
  - config: Pool configuration parameters

Returns:
  - *Q: A new quantum pool instance
*/
func NewQ(ctx context.Context, minWorkers, maxWorkers int, config *Config) *Q {
	ctx, cancel := context.WithCancel(ctx)
	q := &Q{
		ctx:        ctx,
		cancel:     cancel,
		breakers:   make(map[string]*CircuitBreaker),
		workerList: make([]*Worker, 0),
		quit:       make(chan struct{}),
		jobs:       make(chan Job, maxWorkers*10),
		workers:    make(chan chan Job, maxWorkers),
		space:      NewQSpace(),
		metrics:    NewMetrics(),
		config:     config,
	}

	// Start initial workers
	for i := 0; i < minWorkers; i++ {
		q.startWorker()
	}

	// Start the manager goroutine
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		q.manage()
	}()

	// Start metrics collection
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		q.collectMetrics()
	}()

	// Start scaler with appropriate configuration
	scalerConfig := &ScalerConfig{
		TargetLoad:         2.0,                    // Reasonable target load
		ScaleUpThreshold:   4.0,                    // Scale up when load is high
		ScaleDownThreshold: 1.0,                    // Scale down when load is low
		Cooldown:           time.Millisecond * 500, // Reasonable cooldown
	}
	q.scaler = NewScaler(q, minWorkers, maxWorkers, scalerConfig)

	return q
}

/*
	manage handles the core job scheduling loop of the quantum pool.

It distributes jobs to available workers while respecting timeouts and
maintaining quantum state consistency through QSpace integration.

This method runs as a goroutine and continues until the pool's context is cancelled.
*/
func (q *Q) manage() {
	for {
		select {
		case <-q.ctx.Done():
			return
		case job := <-q.jobs:
			// Wait for a worker with timeout
			select {
			case <-q.ctx.Done():
				return
			case workerChan := <-q.workers:
				// Send job to worker
				select {
				case workerChan <- job:
					// Job successfully sent to worker
				case <-q.ctx.Done():
					return
				}
			case <-time.After(q.getSchedulingTimeout()):
				log.Printf("No available workers for job: %s, timeout occurred", job.ID)
				// Store error result since we couldn't process the job
				q.space.Store(job.ID, nil, []State{{
					Value:       fmt.Errorf("no available workers"),
					Probability: 1.0,
				}}, job.TTL)
			}
		}
	}
}

/*
	collectMetrics collects and updates metrics for the quantum pool.

It periodically updates metrics such as job queue size, active workers, and
other relevant statistics. This method runs as a goroutine and continues until
the pool's context is cancelled.
*/
func (q *Q) collectMetrics() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-q.ctx.Done():
			return
		case <-ticker.C:
			q.metrics.mu.Lock()
			q.metrics.JobQueueSize = len(q.jobs)
			q.metrics.ActiveWorkers = len(q.workers)
			q.metrics.mu.Unlock()
		}
	}
}

/*
	Schedule submits a job to the quantum pool for execution.

The job is processed according to quantum-inspired principles, maintaining
state history and uncertainty levels through QSpace integration.

Parameters:
  - id: Unique identifier for the job
  - fn: The function to execute
  - opts: Optional job configuration parameters

Returns:
  - chan *QValue: Channel that will receive the job's result
*/
func (q *Q) Schedule(id string, fn func() (any, error), opts ...JobOption) chan *QValue {
	// Create context with configured timeout
	ctx, cancel := context.WithTimeout(q.ctx, q.getSchedulingTimeout())
	defer cancel()

	startTime := time.Now()

	job := Job{
		ID: id,
		Fn: fn,
		RetryPolicy: &RetryPolicy{
			MaxAttempts: 3,
			Strategy:    &ExponentialBackoff{Initial: time.Second},
		},
		StartTime: startTime,
	}

	// Apply options
	for _, opt := range opts {
		opt(&job)
	}

	// Check circuit breaker if configured
	if job.CircuitID != "" {
		breaker := q.getCircuitBreaker(job)
		if breaker != nil && !breaker.Allow() {
			ch := make(chan *QValue, 1)
			ch <- &QValue{
				Error:     fmt.Errorf("circuit breaker %s is open", job.CircuitID),
				CreatedAt: time.Now(),
			}
			close(ch)
			return ch
		}
	}

	// Try to schedule job with context timeout
	select {
	case q.jobs <- job:
		// Use the pointer channel directly from QSpace
		return q.space.Await(id)
	case <-ctx.Done():
		ch := make(chan *QValue, 1)
		ch <- &QValue{
			Error:     fmt.Errorf("job scheduling timeout: %w", ctx.Err()),
			CreatedAt: time.Now(),
		}
		close(ch)

		// Update metrics for scheduling failure
		q.metrics.mu.Lock()
		q.metrics.SchedulingFailures++
		q.metrics.mu.Unlock()

		return ch
	}
}

/*
	CreateBroadcastGroup creates a new broadcast group.

Initializes a new broadcast group with specified parameters and default
quantum properties such as minimum uncertainty.
*/
func (q *Q) CreateBroadcastGroup(id string, ttl time.Duration) *BroadcastGroup {
	return q.space.CreateBroadcastGroup(id, ttl)
}

/*
	Subscribe returns a channel for receiving values from a broadcast group.

Provides a channel for receiving quantum values from a specific broadcast group.
*/
func (q *Q) Subscribe(groupID string) chan QValue {
	qvChan := q.space.Subscribe(groupID)
	if qvChan == nil {
		return nil
	}

	resultChan := make(chan QValue, 10)
	go func() {
		defer close(resultChan)
		for qv := range qvChan {
			if qv != nil {
				resultChan <- QValue{
					Value:     qv.Value,
					Error:     qv.Error,
					CreatedAt: qv.CreatedAt,
				}
			}
		}
	}()
	return resultChan
}

/*
	startWorker starts a new worker.

Initializes a new worker and adds it to the pool's worker list.
*/
func (q *Q) startWorker() {
	worker := &Worker{
		pool:   q,
		jobs:   make(chan Job),
		cancel: nil,
	}
	q.workerMu.Lock()
	q.workerList = append(q.workerList, worker)
	q.workerMu.Unlock()

	q.metrics.mu.Lock()
	q.metrics.WorkerCount++
	q.metrics.mu.Unlock()

	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		worker.run()
	}()
	log.Printf("Started worker, total workers: %d", q.metrics.WorkerCount)
}

/*
	WithTTL configures TTL for a job.

Sets the time-to-live (TTL) for a job, which determines how long the job will
remain in the system before being discarded.
*/
func WithTTL(ttl time.Duration) JobOption {
	return func(j *Job) {
		j.TTL = ttl
	}
}

/*
	getCircuitBreaker returns the circuit breaker for a job.

If the job does not have a circuit ID or configuration, it returns nil.
*/		
func (q *Q) getCircuitBreaker(job Job) *CircuitBreaker {
	if job.CircuitID == "" || job.CircuitConfig == nil {
		return nil
	}

	q.breakersMu.Lock()
	defer q.breakersMu.Unlock()

	breaker, exists := q.breakers[job.CircuitID]
	if !exists {
		breaker = &CircuitBreaker{
			maxFailures:  job.CircuitConfig.MaxFailures,
			resetTimeout: job.CircuitConfig.ResetTimeout,
			halfOpenMax:  job.CircuitConfig.HalfOpenMax,
			state:        CircuitClosed,
		}
		q.breakers[job.CircuitID] = breaker
	}

	return breaker
}

/*
											getSchedulingTimeout returns the scheduling timeout from the configuration or
	uses a default value if not specified.
*/
func (q *Q) getSchedulingTimeout() time.Duration {
	if q.config != nil && q.config.SchedulingTimeout > 0 {
		return q.config.SchedulingTimeout
	}
	return 5 * time.Second // Default timeout
}

/*
	Close gracefully shuts down the quantum pool.

It ensures all workers complete their current jobs and cleans up resources.
The shutdown process:
 1. Cancels the pool's context
 2. Waits for all goroutines to complete
 3. Closes all channels safely
 4. Cleans up worker resources
*/
func (q *Q) Close() {
	if q == nil {
		return
	}

	log.Println("Closing Quantum Pool")

	// Cancel context first to stop all operations
	if q.cancel != nil {
		log.Println("Cancelling context")
		q.cancel()
	}

	// Wait for all goroutines to finish before closing channels
	q.wg.Wait()

	// Now it's safe to close channels as no goroutines are using them
	q.workerMu.Lock()
	for _, worker := range q.workerList {
		close(worker.jobs)
	}
	q.workerList = nil
	q.workerMu.Unlock()

	close(q.quit)
	close(q.jobs)
	close(q.workers)

	log.Println("Quantum Pool closed")
}
