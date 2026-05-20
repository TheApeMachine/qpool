package qpool

import (
	"math"
	"sync/atomic"
	"time"
)

/*
BackPressureRegulator implements Regulator using atomic pressure derived from MetricReading.
*/
type BackPressureRegulator struct {
	maxQueueSize      int
	targetProcessTime time.Duration
	currentPressure   atomic.Uint64
	lastReading       atomic.Pointer[MetricReading]
}

/*
NewBackPressureRegulator constructs a back-pressure regulator.
*/
func NewBackPressureRegulator(
	maxQueueSize int, targetProcessTime, pressureWindow time.Duration,
) *BackPressureRegulator {
	_ = pressureWindow

	return &BackPressureRegulator{
		maxQueueSize:      maxQueueSize,
		targetProcessTime: targetProcessTime,
	}
}

/*
Observe recomputes pressure from the supplied reading.
*/
func (bp *BackPressureRegulator) Observe(reading *MetricReading) {
	if reading == nil {
		return
	}

	bp.lastReading.Store(reading)

	queuePressure := 0.0
	if bp.maxQueueSize > 0 {
		queuePressure = float64(reading.JobQueueSize) / float64(bp.maxQueueSize)
	}

	timingPressure := 0.0

	if bp.targetProcessTime > 0 && reading.AverageJobLatency > 0 {
		timingPressure = float64(reading.AverageJobLatency) / float64(bp.targetProcessTime)
	}

	p := queuePressure*0.6 + timingPressure*0.4
	p = min(1.0, max(0.0, p))

	bp.currentPressure.Store(math.Float64bits(p))
}

/*
Limit returns true when intake should be rejected (~80% pressure).
*/
func (bp *BackPressureRegulator) Limit() bool {
	p := math.Float64frombits(bp.currentPressure.Load())

	return p >= 0.8
}

/*
Renormalize slowly bleeds pressure when the queue and latency look healthy.
*/
func (bp *BackPressureRegulator) Renormalize() {
	read := bp.lastReading.Load()
	if read == nil {
		return
	}

	if bp.maxQueueSize > 0 &&
		read.JobQueueSize < bp.maxQueueSize/2 &&
		read.AverageJobLatency < bp.targetProcessTime {

		cur := math.Float64frombits(bp.currentPressure.Load())
		next := max(0.0, cur-0.1)
		bp.currentPressure.Store(math.Float64bits(next))
	}
}

/*
GetPressure returns the latest computed pressure in [0,1].
*/
func (bp *BackPressureRegulator) GetPressure() float64 {
	return math.Float64frombits(bp.currentPressure.Load())
}
