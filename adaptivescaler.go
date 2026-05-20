package qpool

import (
	"math"
	"sync/atomic"
	"time"
)

/*
AdaptiveScalerRegulator implements Regulator by scaling the pool when Observed readings cross thresholds.
*/
type AdaptiveScalerRegulator struct {
	pool               *Q
	minWorkers         int
	maxWorkers         int
	targetLoad         float64
	scaleUpThreshold   float64
	scaleDownThreshold float64
	cooldown           time.Duration
	lastScaleUnixNano  atomic.Int64
	reading            atomic.Pointer[MetricReading]
}

/*
NewAdaptiveScalerRegulator constructs an adaptive scaler regulator.
*/
func NewAdaptiveScalerRegulator(
	pool *Q, minWorkers, maxWorkers int, config *ScalerConfig,
) *AdaptiveScalerRegulator {
	as := &AdaptiveScalerRegulator{
		pool:               pool,
		minWorkers:         minWorkers,
		maxWorkers:         maxWorkers,
		targetLoad:         config.TargetLoad,
		scaleUpThreshold:   config.ScaleUpThreshold,
		scaleDownThreshold: config.ScaleDownThreshold,
		cooldown:           config.Cooldown,
	}

	as.lastScaleUnixNano.Store(time.Now().UnixNano())

	return as
}

/*
Observe refreshes metrics and may scale the worker count.
*/
func (as *AdaptiveScalerRegulator) Observe(reading *MetricReading) {
	if reading != nil {
		as.reading.Store(reading)
	}

	as.evaluate()
}

/*
Limit mirrors scaler saturation semantics when already at max workers under extreme load.
*/
func (as *AdaptiveScalerRegulator) Limit() bool {
	read := as.reading.Load()
	if read == nil {
		return false
	}

	if read.WorkerCount >= as.maxWorkers {
		load := float64(read.JobQueueSize) / float64(max(1, read.WorkerCount))

		return load > as.scaleUpThreshold
	}

	return false
}

/*
Renormalize retries scaling evaluation after the cooldown window.
*/
func (as *AdaptiveScalerRegulator) Renormalize() {
	last := as.lastScaleUnixNano.Load()
	if time.Since(time.Unix(0, last)) >= as.cooldown {
		as.evaluate()
	}
}

func (as *AdaptiveScalerRegulator) evaluate() {
	read := as.reading.Load()
	if read == nil {
		return
	}

	last := time.Unix(0, as.lastScaleUnixNano.Load())
	if time.Since(last) < as.cooldown {
		return
	}

	workers := read.WorkerCount
	if workers <= 0 {
		workers = 1
	}

	tl := as.targetLoad

	if tl <= 0 {
		tl = 1
	}

	currentLoad := float64(read.JobQueueSize) / float64(workers)

	switch {
	case currentLoad > as.scaleUpThreshold && read.WorkerCount < as.maxWorkers:
		needed := int(math.Ceil(float64(read.JobQueueSize) / tl))
		delta := max(0, needed-read.WorkerCount)
		toAdd := min(as.maxWorkers-read.WorkerCount, delta)

		if toAdd > 0 {
			as.scaleUp(toAdd)
			as.lastScaleUnixNano.Store(time.Now().UnixNano())
			as.pool.metrics.NoteLastScale(time.Now())
		}

	case currentLoad < as.scaleDownThreshold && read.WorkerCount > as.minWorkers:
		needed := max(int(math.Ceil(float64(read.JobQueueSize)/tl)), as.minWorkers)

		toRemove := min(
			read.WorkerCount-as.minWorkers,
			max(1, (read.WorkerCount-needed)/2),
		)

		if toRemove > 0 {
			as.pool.scaleDownWorkers(toRemove)
			as.lastScaleUnixNano.Store(time.Now().UnixNano())
			as.pool.metrics.NoteLastScale(time.Now())
		}
	}
}

func (as *AdaptiveScalerRegulator) scaleUp(count int) {
	if count <= 0 {
		return
	}

	curWorkers := int(as.pool.metrics.workerCount.Load())

	toAdd := min(as.maxWorkers-curWorkers, count)

	for range toAdd {
		as.pool.startWorker()
	}
}
