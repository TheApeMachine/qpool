package qpool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/phuslu/log"
)

/*
Q combines a buffered job queue, fixed worker set, optional regulators, and result tracking via QSpace.
*/
type Q struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	jobCh        chan Job
	shutdownMu   sync.RWMutex
	stopping     atomic.Bool
	closeJobOnce sync.Once
	minWorkers   int
	maxWorkers   int

	space   *QSpace
	scaler  *Scaler
	metrics *Metrics

	breakers   *circuitBreakerCache
	registry   *workerRegistry
	nextWorker atomic.Uint64

	config *Config
}

/*
NewQ constructs a pool with minWorkers..maxWorkers goroutines competing on a shared job channel.
*/
func NewQ(ctx context.Context, minWorkers, maxWorkers int, config *Config) *Q {
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

	q := &Q{
		ctx:        ctx,
		cancel:     cancel,
		jobCh:      make(chan Job, capacity),
		minWorkers: minWorkers,
		maxWorkers: maxWorkers,
		space:      NewQSpace(),
		metrics:    NewMetrics(),
		breakers:   newCircuitBreakerCache(config.CircuitBreakerLimit),
		registry:   newWorkerRegistry(),
		config:     config,
	}

	for i := 0; i < minWorkers; i++ {
		q.startWorker()
	}

	if config.Scaler != nil {
		q.scaler = NewScaler(q, minWorkers, maxWorkers, config.Scaler)
	}

	return q
}

/*
MetricSnapshot returns a point-in-time copy of atomic pool counters (workers,
busy workers, queue depth, and regulator-facing fields).
*/
func (q *Q) MetricSnapshot() MetricReading {
	if q == nil {
		return MetricReading{}
	}

	r := q.metrics.CollectReading()

	return *r
}

/*
WorkerBounds returns the configured minimum and maximum worker goroutine counts.
*/
func (q *Q) WorkerBounds() (minWorkers, maxWorkers int) {
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
func (q *Q) PeriodicScalerConfigured() bool {
	return q != nil && q.config != nil && q.config.Scaler != nil
}

func (q *Q) publishTelemetry(ev Event) {
	if q != nil && q.config != nil && q.config.TelemetryPublish != nil {
		q.config.TelemetryPublish(ev)

		return
	}

	Publish(ev)
}

func (q *Q) schedulingTimeout() time.Duration {
	if q.config != nil && q.config.SchedulingTimeout > 0 {
		return q.config.SchedulingTimeout
	}

	return 5 * time.Second
}

func errorFuture(err error) chan *QValue {
	ch := make(chan *QValue, 1)
	qv := NewQValue(nil)
	qv.Error = err
	ch <- qv

	close(ch)

	return ch
}

func (q *Q) scheduleDoneError(ctx context.Context) (error, bool) {
	if err := q.ctx.Err(); err != nil {
		return fmt.Errorf("qpool: pool closed: %w", err), false
	}

	return fmt.Errorf("job scheduling timeout: %w", ctx.Err()), true
}

func (q *Q) enqueueJob(ctx context.Context, job Job) error {
	if q.stopping.Load() {
		return fmt.Errorf("qpool: pool closed")
	}

	q.shutdownMu.RLock()
	defer q.shutdownMu.RUnlock()

	if q.stopping.Load() {
		return fmt.Errorf("qpool: pool closed")
	}

	if err := q.ctx.Err(); err != nil {
		return fmt.Errorf("qpool: pool closed: %w", err)
	}

	select {
	case <-q.ctx.Done():
		return fmt.Errorf("qpool: pool closed: %w", q.ctx.Err())
	case q.jobCh <- job:
		q.publishTelemetry(Event{
			Component: "qpool",
			Op:        "schedule",
			Message:   fmt.Sprintf("job scheduled: %s", job.ID),
			Time:      time.Now(),
			Level:     log.InfoLevel,
		})

		q.metrics.incJobQueued()

		return nil
	case <-ctx.Done():
		err, schedulingFailure := q.scheduleDoneError(ctx)

		if schedulingFailure {
			q.metrics.incSchedulingFailure()
		}

		return err
	}
}

/*
Schedule enqueues a job when regulators and optional circuit breaker permit.
Results arrive on the returned channel backed by QSpace. The job id doubles as
the result key until TTL expires — reuse the same id for a logically new piece
of work while older results remain queued and callers will unblock with the
stale completion first unless result cleanup removed it first.
*/
func (q *Q) Schedule(
	id string,
	fn func(context.Context) (any, error),
	opts ...JobOption,
) chan *QValue {
	ctx, cancel := context.WithTimeout(q.ctx, q.schedulingTimeout())
	defer cancel()

	startTime := time.Now()

	job := Job{
		ID:        id,
		Fn:        fn,
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

				return errorFuture(fmt.Errorf("qpool: regulator rejected schedule"))
			}
		}
	}

	if job.CircuitID != "" {
		breaker := q.breakerFor(&job)

		if breaker != nil && !breaker.Allow() {
			return errorFuture(fmt.Errorf("circuit breaker %s is open", job.CircuitID))
		}

		if breaker != nil {
			job.circuitBreaker = breaker
		}
	}

	if q.stopping.Load() {
		return errorFuture(fmt.Errorf("qpool: pool closed"))
	}

	if len(job.Dependencies) > 0 {
		if err := q.startDependencyWait(job); err != nil {
			return errorFuture(err)
		}

		return q.space.Await(id)
	}

	if err := q.enqueueJob(ctx, job); err != nil {
		return errorFuture(err)
	}

	return q.space.Await(id)
}

/*
CreateBroadcastGroup allocates a group stored inside QSpace.
*/
func (q *Q) CreateBroadcastGroup(id string, ttl time.Duration) *BroadcastGroup {
	return q.space.CreateBroadcastGroup(id, ttl)
}

/*
Subscribe returns the broadcast group's subscriber channel for groupID.
*/
func (q *Q) Subscribe(groupID string) chan *QValue {
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
func (q *Q) PeekResult(id string) (*QValue, bool) {
	if q == nil {
		return nil, false
	}

	return q.space.PeekResult(id)
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
