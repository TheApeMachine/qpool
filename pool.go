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
	workers    chan chan Job
	jobs       chan Job
	space      *QuantumSpace
	scaler     *Scaler
	metrics    *Metrics
	breakers   map[string]*CircuitBreaker
	quit       chan struct{}
	workerMu   sync.Mutex
	workerList []*Worker
}

// NewQ creates a new quantum pool
func NewQ(ctx context.Context, minWorkers, maxWorkers int) *Q {
	q := &Q{
		ctx:        ctx,
		workers:    make(chan chan Job, maxWorkers),
		jobs:       make(chan Job, maxWorkers*10),
		space:      newQuantumSpace(),
		metrics:    newMetrics(),
		breakers:   make(map[string]*CircuitBreaker),
		quit:       make(chan struct{}),
		workerList: []*Worker{},
	}

	// Initialize scaler
	q.scaler = &Scaler{
		pool:               q,
		minWorkers:         minWorkers,
		maxWorkers:         maxWorkers,
		targetLoad:         0.7,
		scaleUpThreshold:   0.8,
		scaleDownThreshold: 0.3,
		cooldown:           5 * time.Second,
	}

	// Start initial workers
	for i := 0; i < minWorkers; i++ {
		q.startWorker()
	}

	// Start pool management
	go q.manage()
	go q.collectMetrics()

	return q
}

// Pool management
func (q *Q) manage() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-q.ctx.Done():
			q.workerMu.Lock()
			// First close the jobs channel to prevent new jobs
			close(q.jobs)
			// Then cancel all workers
			for _, w := range q.workerList {
				w.cancel()
			}
			q.workerMu.Unlock()
			close(q.quit)
			return
		case <-ticker.C:
			q.scaler.evaluate()
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

	select {
	case q.jobs <- job:
		return q.space.Await(id)
	case <-time.After(5 * time.Second): // Add timeout
		ch := make(chan QuantumValue, 1)
		ch <- QuantumValue{Error: fmt.Errorf("job scheduling timeout")}
		close(ch)
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
	workerCtx, cancel := context.WithCancel(q.ctx)
	w := &Worker{
		pool:   q,
		jobs:   make(chan Job),
		cancel: cancel,
	}
	q.workerMu.Lock()
	q.workerList = append(q.workerList, w)
	q.workerMu.Unlock()

	q.metrics.mu.Lock()
	q.metrics.WorkerCount++
	q.metrics.mu.Unlock()
	go w.start(workerCtx)
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
	q := NewQ(ctx, 5, 20)

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
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
