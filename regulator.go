package qpool

import "time"

/*
MetricReading is a point-in-time snapshot of pool metrics for regulators.

It carries plain fields only so Observe implementations never capture mutex-backed state.
*/
type MetricReading struct {
	WorkerCount         int
	BusyWorkers         int
	JobQueueSize        int
	AverageJobLatency   time.Duration
	P95JobLatency       time.Duration
	P99JobLatency       time.Duration
	JobSuccessRate      float64
	ResourceUtilization float64
	TotalJobs           int64
	FailedJobs          int64
	SchedulingFailures  int64
	RateLimitHits       int64
	ThrottledJobs       int64
}

/*
Regulator defines optional admission control and tuning hooks for Q.

Implementations must not retain the MetricReading pointer across goroutines without
copying fields; the pool passes a freshly allocated reading per Schedule tick when needed.
*/
type Regulator interface {
	Observe(reading *MetricReading)
	Limit() bool
	Renormalize()
}

/*
NewRegulator returns r unchanged (helper for readable construction lists).
*/
func NewRegulator(r Regulator) Regulator {
	return r
}
