package qpool

import (
	"context"
	"fmt"
	"log"
	"math"
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

// QuantumValue wraps a value with metadata
type QuantumValue struct {
	Value     any
	Error     error
	CreatedAt time.Time
	TTL       time.Duration
}

// QuantumSpace handles value storage and messaging
type QuantumSpace struct {
	mu       sync.RWMutex
	values   map[string]*QuantumValue
	waiting  map[string][]chan QuantumValue
	errors   map[string]error
	children map[string][]string
	groups   map[string]*BroadcastGroup
}

// Job represents work to be done
type Job struct {
	ID                    string
	Fn                    func() (any, error)
	RetryPolicy           *RetryPolicy
	CircuitID             string
	Dependencies          []string
	TTL                   time.Duration
	Attempt               int
	LastError             error
	DependencyRetryPolicy *RetryPolicy
}

// Metrics tracks pool performance
type Metrics struct {
	mu            sync.RWMutex
	WorkerCount   int
	JobQueueSize  int
	ActiveWorkers int
	LastScale     time.Time
	ErrorRates    map[string]float64
	TotalJobTime  time.Duration
	JobCount      int64
}

// Scaler manages pool size
type Scaler struct {
	pool               *Q
	minWorkers         int
	maxWorkers         int
	targetLoad         float64
	scaleUpThreshold   float64
	scaleDownThreshold float64
	cooldown           time.Duration
}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaxAttempts int
	Strategy    RetryStrategy
	BackoffFunc func(attempt int) time.Duration
	Filter      func(error) bool
}

// CircuitBreaker prevents cascading failures
type CircuitBreaker struct {
	mu           sync.RWMutex
	failures     int
	lastFailure  time.Time
	state        CircuitState
	maxFailures  int
	resetTimeout time.Duration
	halfOpenMax  int
	halfOpenPass int
}

type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// BroadcastGroup handles pub/sub
type BroadcastGroup struct {
	ID       string
	channels []chan QuantumValue
	TTL      time.Duration
	LastUsed time.Time
}

// RetryStrategy defines the interface for retry behavior
type RetryStrategy interface {
	NextDelay(attempt int) time.Duration
}

// ExponentialBackoff implements RetryStrategy
type ExponentialBackoff struct {
	Initial time.Duration
}

func (eb *ExponentialBackoff) NextDelay(attempt int) time.Duration {
	return eb.Initial * time.Duration(math.Pow(2, float64(attempt-1)))
}

// JobOption is a function type for configuring jobs
type JobOption func(*Job)

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

func newQuantumSpace() *QuantumSpace {
	qs := &QuantumSpace{
		values:   make(map[string]*QuantumValue),
		waiting:  make(map[string][]chan QuantumValue),
		errors:   make(map[string]error),
		children: make(map[string][]string),
		groups:   make(map[string]*BroadcastGroup),
	}

	go qs.cleanup()
	return qs
}

func newMetrics() *Metrics {
	return &Metrics{
		ErrorRates: make(map[string]float64),
	}
}

// Pool management
func (q *Q) manage() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-q.ctx.Done():
			q.workerMu.Lock()
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
	job := Job{
		ID: id,
		Fn: fn,
		RetryPolicy: &RetryPolicy{
			MaxAttempts: 3,
			Strategy:    &ExponentialBackoff{Initial: time.Second},
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(&job)
	}

	q.jobs <- job
	return q.space.Await(id)
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

// WithRetry configures retry behavior for a job
func WithRetry(attempts int, strategy RetryStrategy) JobOption {
	return func(j *Job) {
		j.RetryPolicy = &RetryPolicy{
			MaxAttempts: attempts,
			Strategy:    strategy,
		}
	}
}

// WithCircuitBreaker configures circuit breaker for a job
func WithCircuitBreaker(id string, maxFailures int, resetTimeout time.Duration) JobOption {
	return func(j *Job) {
		j.CircuitID = id
	}
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
