package qpool

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Q is our hybrid worker pool/message queue
type Q struct {
	ctx        context.Context
	cancel     context.CancelFunc
	quit       chan struct{}
	wg         sync.WaitGroup
	workers    chan chan Job
	jobs       chan Job
	space      *QuantumSpace
	scaler     *Scaler
	metrics    *Metrics
	breakers   map[string]*CircuitBreaker
	workerMu   sync.Mutex
	workerList []*Worker
	breakersMu sync.RWMutex
	config     *Config
}

// NewQ creates a new quantum pool
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
		space:      newQuantumSpace(),
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

// Pool management
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
				q.space.Store(job.ID, nil, fmt.Errorf("no available workers"), job.TTL)
			}
		}
	}
}

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

// Public API methods
func (q *Q) Schedule(id string, fn func() (any, error), opts ...JobOption) chan QuantumValue {
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
			ch := make(chan QuantumValue, 1)
			ch <- QuantumValue{
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
		return q.space.Await(id)
	case <-ctx.Done():
		ch := make(chan QuantumValue, 1)
		ch <- QuantumValue{
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

func (q *Q) CreateBroadcastGroup(id string, ttl time.Duration) *BroadcastGroup {
	return q.space.CreateBroadcastGroup(id, ttl)
}

func (q *Q) Subscribe(groupID string) chan QuantumValue {
	return q.space.Subscribe(groupID)
}

// Helper functions
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

// WithTTL configures TTL for a job
func WithTTL(ttl time.Duration) JobOption {
	return func(j *Job) {
		j.TTL = ttl
	}
}

// Example demonstrates usage of the quantum pool
func Example() {
	ctx := context.Background()
	q := NewQ(ctx, 5, 20, nil)

	// Create a broadcast group
	group := q.CreateBroadcastGroup("sensors", time.Minute)
	subscriber1 := q.Subscribe("sensors")
	subscriber2 := q.Subscribe("sensors")

	// Use the subscribers to avoid unused variable warnings
	go func() {
		for value := range subscriber1 {
			// Handle sensor data from subscriber 1
			fmt.Printf("Subscriber 1 received: %v\n", value.Value)
		}
	}()

	go func() {
		for value := range subscriber2 {
			// Handle sensor data from subscriber 2
			fmt.Printf("Subscriber 2 received: %v\n", value.Value)
		}
	}()

	// Simulate broadcasting data
	go func() {
		for {
			group.Send(QuantumValue{Value: "sensor data", CreatedAt: time.Now()})
			time.Sleep(time.Second)
		}
	}()

	// Schedule a job with retry and circuit breaker
	result := q.Schedule("sensor-read", func() (any, error) {
		// Simulate sensor reading
		return "sensor reading", nil
	},
		WithRetry(3, &ExponentialBackoff{Initial: time.Second}),
		WithCircuitBreaker("sensors", 5, time.Minute),
		WithTTL(time.Minute),
	)

	// Process results
	go func() {
		for value := range result {
			if value.Error != nil {
				fmt.Printf("Error: %v\n", value.Error)
				continue
			}
			// Process the sensor data
			fmt.Printf("Processed data: %v\n", value.Value)
		}
	}()

	// Close subscribers after some time (for demonstration purposes)
	time.AfterFunc(10*time.Second, func() {
		close(subscriber1)
		close(subscriber2)
	})
}

// Helper functions
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

// Add method to get scheduling timeout from config or use default
func (q *Q) getSchedulingTimeout() time.Duration {
	if q.config != nil && q.config.SchedulingTimeout > 0 {
		return q.config.SchedulingTimeout
	}
	return 5 * time.Second // Default timeout
}

// Update Close method to handle channel closing safely
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
