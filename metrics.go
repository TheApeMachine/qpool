package qpool

import (
	"math"
	"sync/atomic"
	"time"
)

/*
Metrics tracks pool statistics using atomic operations only.
*/
type Metrics struct {
	workerCount        atomic.Int64
	busyWorkers        atomic.Int64
	jobQueueDepth      atomic.Int64
	schedulingFailures atomic.Int64
	jobCount           atomic.Int64
	failureCount       atomic.Int64
	totalLatencyNs     atomic.Uint64
	maxLatencyNs       atomic.Uint64
	rateLimitHits      atomic.Int64
	throttledJobs      atomic.Int64
	lastScaleUnixNano  atomic.Int64
	resourceUtilBits   atomic.Uint64
}

/*
NewMetrics creates an initialized Metrics holder.
*/
func NewMetrics() *Metrics {
	return &Metrics{}
}

/*
CollectReading builds a regulator-facing snapshot from current atomic counters.
*/
func (m *Metrics) CollectReading() *MetricReading {
	jc := m.jobCount.Load()
	fc := m.failureCount.Load()

	if fc > jc {
		fc = jc
	}

	totalNs := m.totalLatencyNs.Load()

	var avg time.Duration

	if jc > 0 {
		avg = time.Duration(totalNs / uint64(jc))
	}

	var successRate float64

	if jc > 0 {
		successRate = float64(jc-fc) / float64(jc)
		successRate = math.Max(0, math.Min(1, successRate))
	}

	/*
		P95/P99 are not computed from current counters (only max latency is tracked on Metrics).
		These stay zero until ingestion feeds a quantile structure.
	*/
	wc := int(m.workerCount.Load())
	busy := int(m.busyWorkers.Load())

	if busy > wc {
		busy = wc
	}

	if busy < 0 {
		busy = 0
	}

	return &MetricReading{
		WorkerCount:         wc,
		BusyWorkers:         busy,
		JobQueueSize:        int(m.jobQueueDepth.Load()),
		AverageJobLatency:   avg,
		P95JobLatency:       0,
		P99JobLatency:       0,
		JobSuccessRate:      successRate,
		ResourceUtilization: math.Float64frombits(m.resourceUtilBits.Load()),
		TotalJobs:           jc,
		FailedJobs:          fc,
		SchedulingFailures:  m.schedulingFailures.Load(),
		RateLimitHits:       m.rateLimitHits.Load(),
		ThrottledJobs:       m.throttledJobs.Load(),
	}
}

/*
ExportMetrics exports counters as a map for observability.
*/
func (m *Metrics) ExportMetrics() map[string]interface{} {
	r := m.CollectReading()

	return map[string]interface{}{
		"worker_count":         r.WorkerCount,
		"busy_workers":         r.BusyWorkers,
		"queue_size":           r.JobQueueSize,
		"success_rate":         r.JobSuccessRate,
		"avg_latency_ms":       r.AverageJobLatency.Milliseconds(),
		"max_latency_ms":       time.Duration(m.maxLatencyNs.Load()).Milliseconds(),
		"p95_latency_ms":       r.P95JobLatency.Milliseconds(),
		"p99_latency_ms":       r.P99JobLatency.Milliseconds(),
		"resource_utilization": r.ResourceUtilization,
		"last_scale_unix_nano": m.lastScaleUnixNano.Load(),
	}
}

func (m *Metrics) decWorkerCount() {
	m.workerCount.Add(-1)
}

func (m *Metrics) incBusyWorker() {
	m.busyWorkers.Add(1)
}

func (m *Metrics) decBusyWorker() {
	m.busyWorkers.Add(-1)
}

/*
tryIncWorkerIfBelow increments workerCount when it is strictly less than max; returns whether the increment succeeded.
*/
func (m *Metrics) tryIncWorkerIfBelow(max int) bool {
	for {
		cur := m.workerCount.Load()

		if int(cur) >= max {
			return false
		}

		if m.workerCount.CompareAndSwap(cur, cur+1) {
			return true
		}
	}
}

func (m *Metrics) incJobQueued() {
	m.jobQueueDepth.Add(1)
}

func (m *Metrics) decJobQueued() {
	m.jobQueueDepth.Add(-1)
}

func (m *Metrics) incSchedulingFailure() {
	m.schedulingFailures.Add(1)
}

func (m *Metrics) incThrottled() {
	m.throttledJobs.Add(1)
}

/*
RecordJobOutcome records one finished attempt (success or failure) with observed latency.
*/
func (m *Metrics) RecordJobOutcome(latency time.Duration, success bool) {
	m.jobCount.Add(1)

	if !success {
		m.failureCount.Add(1)
	}

	nsInt := latency.Nanoseconds()

	if nsInt < 0 {
		nsInt = 0
	}

	ns := uint64(nsInt)
	m.totalLatencyNs.Add(ns)

	for {
		cur := m.maxLatencyNs.Load()
		if ns <= cur {
			break
		}

		if m.maxLatencyNs.CompareAndSwap(cur, ns) {
			break
		}
	}
}

/*
RecordJobSuccess records a successfully finished job with latency.
*/
func (m *Metrics) RecordJobSuccess(latency time.Duration) {
	m.RecordJobOutcome(latency, true)
}

/*
RecordJobFailure records a failed job without recording a latency sample (does not update totals used for averages).
*/
func (m *Metrics) RecordJobFailure() {
	m.jobCount.Add(1)
	m.failureCount.Add(1)
}

/*
RecordJobExecution records timing from legacy call sites.
*/
func (m *Metrics) RecordJobExecution(startTime time.Time, success bool) {
	m.RecordJobOutcome(time.Since(startTime), success)
}

/*
SetResourceUtilization stores CPU or synthetic utilization for ResourceGovernor (0–1).
Values are clamped to that range; NaN is stored as 0.
*/
func (m *Metrics) SetResourceUtilization(u float64) {
	if u != u {
		u = 0
	} else {
		u = math.Max(0, math.Min(1, u))
	}

	m.resourceUtilBits.Store(math.Float64bits(u))
}

/*
NoteLastScale records scaler activity time (wall clock).
*/
func (m *Metrics) NoteLastScale(t time.Time) {
	m.lastScaleUnixNano.Store(t.UnixNano())
}
