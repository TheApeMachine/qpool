package qpool

import (
	"errors"
	"sync/atomic"
	"time"
)

/*
ErrNoAvailableWorkers is returned when no workers are available to process a job.
*/
var ErrNoAvailableWorkers = errors.New("no workers available to process job")

/*
LoadBalancer implements Regulator using queue depth per worker versus a capacity budget.
*/
type LoadBalancer struct {
	perWorkerCapacity int
	lastReading       atomic.Pointer[MetricReading]
}

/*
NewLoadBalancer constructs a load balancer.

workerCount is ignored (discarded for API compatibility); live worker and queue
values used by Limit and SelectWorker come from Observe readings.
*/
func NewLoadBalancer(workerCount, workerCapacity int) *LoadBalancer {
	_ = workerCount

	return &LoadBalancer{
		perWorkerCapacity: workerCapacity,
	}
}

/*
Observe refreshes the reading used by Limit.
*/
func (lb *LoadBalancer) Observe(reading *MetricReading) {
	if reading != nil {
		lb.lastReading.Store(reading)
	}
}

/*
Limit returns true when every worker segment appears saturated.
*/
func (lb *LoadBalancer) Limit() bool {
	read := lb.lastReading.Load()
	if read == nil {
		return false
	}

	workers := read.WorkerCount
	if workers <= 0 {
		workers = 1
	}

	load := float64(read.JobQueueSize) / float64(workers)
	capacity := float64(lb.perWorkerCapacity)

	return load >= capacity
}

/*
Renormalize is a no-op.
*/
func (lb *LoadBalancer) Renormalize() {}

/*
SelectWorker is retained for API compatibility; routing uses the shared job channel.
*/
func (lb *LoadBalancer) SelectWorker() (int, error) {
	read := lb.lastReading.Load()
	if read == nil {
		return -1, ErrNoAvailableWorkers
	}

	if read.WorkerCount <= 0 {
		return -1, ErrNoAvailableWorkers
	}

	return int(read.TotalJobs % int64(read.WorkerCount)), nil
}

/*
RecordJobStart is a no-op under channel-based dispatch.
*/
func (lb *LoadBalancer) RecordJobStart(workerID int) {
	_ = workerID
}

/*
RecordJobComplete is a no-op under channel-based dispatch.
*/
func (lb *LoadBalancer) RecordJobComplete(workerID int, duration time.Duration) {
	_ = workerID
	_ = duration
}
