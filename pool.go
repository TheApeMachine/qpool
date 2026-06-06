package qpool

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/phuslu/log"
)

type (
	slotFunc[T any] struct {
		threadPtr unsafe.Pointer
		data      T
	}

	dataItem[T any] struct {
		next  atomic.Pointer[dataItem[T]]
		value *slotFunc[T]
	}
)

/*
Q combines a disruptor-backed job queue, fixed worker set, optional regulators, and result tracking via QSpace.
*/
type Q[T any] struct {
	ctx        context.Context
	cancel     context.CancelFunc
	_p1        [cacheLinePadSize - unsafe.Sizeof(uint64(0))]byte
	alloc      func() any
	free       func(any)
	task       func(T)
	_p2        [cacheLinePadSize - unsafe.Sizeof(uint64(0)) - 3*unsafe.Sizeof(func() {})]byte
	top        atomic.Pointer[dataItem[T]]
	_p3        [cacheLinePadSize - unsafe.Sizeof(atomic.Pointer[dataItem[T]]{})]byte
	deps       atomicWaitGroup
	scalerWG   atomicWaitGroup
	jobQueue   *jobDisruptorQueue
	fastQueue  *jobDisruptorQueue
	stopping   atomic.Bool
	minWorkers int
	maxWorkers int
	space      *QSpace
	scaler     *Scaler
	metrics    *Metrics
	breakers   *circuitBreakerCache
	registry   *workerRegistry
	nextWorker atomic.Uint64
	config     *Config
}

/*
NewQ constructs a pool with minWorkers..maxWorkers active disruptor workers.
*/
func NewQ[T any](ctx context.Context, minWorkers, maxWorkers int, config *Config) *Q[T] {
	if config == nil {
		config = NewConfig()
	}

	if maxWorkers < 1 {
		maxWorkers = 1
	}

	if minWorkers < 1 {
		minWorkers = 1
	}

	if minWorkers > maxWorkers {
		minWorkers = maxWorkers
	}

	ctx, cancel := context.WithCancel(ctx)

	capacity := maxWorkers * 10

	if config.JobChannelCapacity > 0 {
		capacity = config.JobChannelCapacity
	}

	q := &Q[T]{
		ctx:        ctx,
		cancel:     cancel,
		minWorkers: minWorkers,
		maxWorkers: maxWorkers,
		space:      NewQSpace(),
		metrics:    NewMetrics(),
		breakers:   newCircuitBreakerCache(config.CircuitBreakerLimit),
		registry:   newWorkerRegistry(),
		config:     config,
	}

	jobQueue, err := newJobDisruptorQueue(qAny(q), capacity, maxWorkers)
	if err != nil {
		cancel()
		panic(err)
	}

	fastQueue, err := newJobDisruptorQueue(qAny(q), maxWorkers*16, maxWorkers)
	if err != nil {
		jobQueue.Close()
		cancel()
		panic(err)
	}

	q.jobQueue = jobQueue
	q.fastQueue = fastQueue
	q.fastQueue.setActiveWorkers(int64(maxWorkers))

	for i := 0; i < minWorkers; i++ {
		q.startWorker()
	}

	if config.Scaler != nil {
		q.scaler = NewScaler(qAny(q), minWorkers, maxWorkers, config.Scaler)
	}

	return q
}

/*
Close shuts down the pool.
*/
func (q *Q[T]) Close() {
	q.closePool()
}

/*
MetricSnapshot returns a point-in-time copy of atomic pool counters (workers,
busy workers, queue depth, and regulator-facing fields).
*/
func (q *Q[T]) MetricSnapshot() MetricReading {
	if q == nil {
		return MetricReading{}
	}

	r := q.metrics.CollectReading()

	return *r
}

/*
WorkerBounds returns the configured minimum and maximum worker goroutine counts.
*/
func (q *Q[T]) WorkerBounds() (minWorkers, maxWorkers int) {
	if q == nil {
		return 0, 0
	}

	return q.minWorkers, q.maxWorkers
}

/*
PeriodicScalerConfigured reports whether NewQ wired the built-in
interval scaler. Adaptive admission regulators may resize the pool
independently; those are not mirrored here.
*/
func (q *Q[T]) PeriodicScalerConfigured() bool {
	return q != nil && q.config != nil && q.config.Scaler != nil
}

func (q *Q[T]) publishTelemetry(ev Event) {
	if q != nil && q.config != nil && q.config.TelemetryPublish != nil {
		q.config.TelemetryPublish(ev)

		return
	}

	Publish(ev)
}

func (q *Q[T]) schedulingTimeout() time.Duration {
	if q.config != nil && q.config.SchedulingTimeout > 0 {
		return q.config.SchedulingTimeout
	}

	return 5 * time.Second
}

func (q *Q[T]) scheduleDoneError(ctx context.Context) (error, bool) {
	if err := q.ctx.Err(); err != nil {
		return fmt.Errorf("qpool: pool closed: %w", err), false
	}

	return fmt.Errorf("job scheduling timeout: %w", ctx.Err()), true
}

func (q *Q[T]) enqueueJob(ctx context.Context, job Job) error {
	if q.stopping.Load() {
		return fmt.Errorf("qpool: pool closed")
	}

	if q.stopping.Load() {
		return fmt.Errorf("qpool: pool closed")
	}

	if err := q.ctx.Err(); err != nil {
		return fmt.Errorf("qpool: pool closed: %w", err)
	}

	if err := q.jobQueue.publishJob(ctx, job); err != nil {
		if q.ctx.Err() != nil {
			return fmt.Errorf("qpool: pool closed: %w", q.ctx.Err())
		}

		if ctx.Err() != nil {
			err, schedulingFailure := q.scheduleDoneError(ctx)
			if schedulingFailure {
				q.metrics.incSchedulingFailure()
			}

			return err
		}

		return fmt.Errorf("qpool: schedule job: %w", err)
	}

	q.publishTelemetry(Event{
		Component: "qpool",
		Op:        "schedule",
		Message:   fmt.Sprintf("job scheduled: %s", job.ID),
		Time:      time.Now(),
		Level:     log.InfoLevel,
	})

	return nil
}

/*
Schedule enqueues a job when regulators and optional circuit breaker permit.
Results arrive on the returned lock-free handle backed by QSpace. The job id doubles as
the result key until TTL expires — reuse the same id for a logically new piece
of work while older results remain queued and callers will unblock with the
stale completion first unless result cleanup removed it first.
*/
func (q *Q[T]) Schedule(
	id string,
	fn func(context.Context) (T, error),
	opts ...JobOption,
) *ResultWait[T] {
	ctx, cancel := context.WithTimeout(q.ctx, q.schedulingTimeout())
	defer cancel()

	startTime := time.Now()

	job := Job{
		ID: id,
		Fn: func(ctx context.Context) (any, error) {
			return fn(ctx)
		},
		StartTime: startTime,
		RetryPolicy: &RetryPolicy{
			MaxAttempts: 1,
			Strategy:    &ExponentialBackoff{Initial: time.Second},
		},
	}

	for _, opt := range opts {
		opt(&job)
	}

	if q.config != nil && len(q.config.Regulators) > 0 {
		reading := q.metrics.CollectReading()

		for _, reg := range q.config.Regulators {
			reg.Observe(reading)
		}

		for _, reg := range q.config.Regulators {
			if reg.Limit() {
				q.metrics.incThrottled()

				return errorResultWait[T](fmt.Errorf("qpool: regulator rejected schedule"))
			}
		}
	}

	if job.CircuitID != "" {
		breaker := q.breakerFor(&job)

		if breaker != nil && !breaker.Allow() {
			return errorResultWait[T](fmt.Errorf("circuit breaker %s is open", job.CircuitID))
		}

		if breaker != nil {
			job.circuitBreaker = breaker
		}
	}

	if q.stopping.Load() {
		return errorResultWait[T](fmt.Errorf("qpool: pool closed"))
	}

	if len(job.Dependencies) > 0 {
		if err := q.startDependencyWait(job); err != nil {
			return errorResultWait[T](err)
		}

		return typedResultWait[T](q.space.Await(id))
	}

	if err := q.enqueueJob(ctx, job); err != nil {
		return errorResultWait[T](err)
	}

	return typedResultWait[T](q.space.Await(id))
}

/*
CreateBroadcastGroup allocates a group stored inside QSpace.
*/
func (q *Q[T]) CreateBroadcastGroup(id string, ttl time.Duration) *BroadcastGroup {
	return q.space.CreateBroadcastGroup(id, ttl)
}

/*
Subscribe returns the broadcast group's lock-free consumer for groupID.
*/
func (q *Q[T]) Subscribe(groupID string) *BroadcastConsumer {
	return q.space.Subscribe(groupID)
}

/*
PeekResult returns a shallow copy of the stored QValue for job id when QSpace holds
a non-expired result. It returns (nil, false) when no result is stored for id, when
TTL expiration or eviction removed the entry, or when the pool or space cannot serve
the query (including during shutdown). The returned *QValue points at a new struct
value copied from the actor's map entry; see QSpace.PeekResult for concurrency and
read-only semantics versus nested reference fields in QValue.
*/
func (q *Q[T]) PeekResult(id string) (*QValue[T], bool) {
	if q == nil {
		return nil, false
	}

	v, ok := q.space.PeekResult(id)

	return qValuePtr[T](v), ok
}

/*
WithTTL sets how long QSpace retains the job result before expiration
cleanup. It does not cap execution time; use WithExecTimeout for that.
*/
func WithTTL(ttl time.Duration) JobOption {
	return func(j *Job) {
		j.TTL = ttl
	}
}

/*
WithExecTimeout sets the per-invocation deadline passed to Fn. Zero selects
the pool Config.SchedulingTimeout default (when positive) or five seconds.
*/
func WithExecTimeout(duration time.Duration) JobOption {
	return func(j *Job) {
		j.ExecTimeout = duration
	}
}

/*
WithDependencyAwaitTimeout sets how long a job waits for each dependency before
its dependency wait attempt times out. It does not add dependencies; combine it
with WithDependencies for dependency-ordered jobs.
*/
func WithDependencyAwaitTimeout(duration time.Duration) JobOption {
	return func(job *Job) {
		if duration <= 0 {
			return
		}

		if job.DependencyRetryPolicy == nil {
			job.DependencyRetryPolicy = &RetryPolicy{
				MaxAttempts: 1,
				Strategy:    &ExponentialBackoff{Initial: time.Second},
			}
		}

		if job.DependencyRetryPolicy.MaxAttempts <= 0 {
			job.DependencyRetryPolicy.MaxAttempts = 1
		}

		if job.DependencyRetryPolicy.Strategy == nil {
			job.DependencyRetryPolicy.Strategy = &ExponentialBackoff{
				Initial: time.Second,
			}
		}

		job.DependencyRetryPolicy.PerAttemptTimeout = duration
	}
}
