package qpool

import (
	"math"
	"sync/atomic"
	"time"

	"github.com/phuslu/log"
)

/*
Scaler periodically evaluates queue depth and scales worker count on load.
It does not reject Schedule; back-pressure is the disruptor queue and optional Regulators.
*/
type Scaler struct {
	pool               *Q[any]
	minWorkers         int
	maxWorkers         int
	targetLoad         float64
	scaleUpThreshold   float64
	scaleDownThreshold float64
	cooldown           time.Duration
	evalInterval       time.Duration
	lastScaleDownNano  atomic.Int64
	reading            atomic.Pointer[MetricReading]
}

/*
ScalerConfig defines configuration for the periodic scaler.
*/
type ScalerConfig struct {
	TargetLoad         float64
	ScaleUpThreshold   float64
	ScaleDownThreshold float64
	Cooldown           time.Duration
	/*
		Interval is the ticker period for CollectReading and evaluate.

		Zero defaults to one second inside NewScaler.
	*/
	Interval time.Duration
}

func (config *ScalerConfig) jobQueueCapacity(maxWorkers int) int {
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	if config == nil {
		return maxWorkers
	}

	threshold := config.ScaleUpThreshold

	if threshold <= 0 {
		threshold = 1
	}

	return int(math.Ceil(float64(maxWorkers) * threshold))
}

/*
Observe refreshes metrics and may scale the worker count up immediately.
*/
func (scaler *Scaler) Observe(reading *MetricReading) {
	if scaler == nil {
		return
	}

	if reading != nil {
		scaler.reading.Store(reading)
	}

	scaler.evaluate()
}

/*
Limit is always false: the scaler grows worker count instead of rejecting schedules.
*/
func (scaler *Scaler) Limit() bool {
	return false
}

/*
Renormalize retries scale-down evaluation after the cooldown window.
*/
func (scaler *Scaler) Renormalize() {
	if scaler == nil {
		return
	}

	last := scaler.lastScaleDownNano.Load()

	if time.Since(time.Unix(0, last)) >= scaler.cooldown {
		scaler.evaluate()
	}
}

func (scaler *Scaler) evaluate() {
	if scaler == nil {
		return
	}

	read := scaler.reading.Load()

	if read == nil {
		read = scaler.pool.metrics.CollectReading()
		scaler.reading.Store(read)
	}

	workers := read.WorkerCount

	if workers <= 0 {
		workers = 1
	}

	targetLoad := scaler.targetLoad

	if targetLoad <= 0 {
		targetLoad = 1
	}

	currentLoad := float64(read.JobQueueSize) / float64(workers)

	if currentLoad > scaler.scaleUpThreshold && read.WorkerCount < scaler.maxWorkers {
		needed := int(math.Ceil(float64(read.JobQueueSize) / targetLoad))
		delta := max(0, needed-read.WorkerCount)
		toAdd := min(scaler.maxWorkers-read.WorkerCount, delta)

		if toAdd > 0 {
			scaler.scaleUp(toAdd)
			scaler.noteScaleUp(toAdd)
		}
	}

	last := time.Unix(0, scaler.lastScaleDownNano.Load())

	if time.Since(last) < scaler.cooldown {
		return
	}

	if currentLoad < scaler.scaleDownThreshold && read.WorkerCount > scaler.minWorkers {
		needed := max(int(math.Ceil(float64(read.JobQueueSize)/targetLoad)), scaler.minWorkers)

		toRemove := min(
			read.WorkerCount-scaler.minWorkers,
			max(1, (read.WorkerCount-needed)/2),
		)

		if toRemove > 0 {
			scaler.pool.scaleDownWorkers(toRemove)
			scaler.noteScaleDown(toRemove)
		}
	}
}

func (scaler *Scaler) scaleUp(count int) {
	if count <= 0 {
		return
	}

	currentWorkers := int(scaler.pool.metrics.workerCount.Load())
	toAdd := min(scaler.maxWorkers-currentWorkers, count)

	for range toAdd {
		scaler.pool.startWorker()
	}
}

func (scaler *Scaler) noteScaleUp(delta int) {
	now := time.Now()

	scaler.pool.metrics.NoteLastScale(now)

	scaler.pool.publishTelemetry(Event{
		Component: "qpool",
		Op:        "scale-up",
		Message:   "scaled worker pool up",
		Time:      now,
		Level:     log.DebugLevel,
		Fields: []Field{
			{Key: "delta", Value: delta},
			{Key: "workers", Value: scaler.pool.metrics.workerCount.Load()},
		},
	})
}

func (scaler *Scaler) noteScaleDown(delta int) {
	now := time.Now()

	scaler.lastScaleDownNano.Store(now.UnixNano())
	scaler.pool.metrics.NoteLastScale(now)

	scaler.pool.publishTelemetry(Event{
		Component: "qpool",
		Op:        "scale-down",
		Message:   "scaled worker pool down",
		Time:      now,
		Level:     log.DebugLevel,
		Fields: []Field{
			{Key: "delta", Value: delta},
			{Key: "workers", Value: scaler.pool.metrics.workerCount.Load()},
		},
	})
}

func (scaler *Scaler) run() {
	interval := scaler.evalInterval

	if interval <= 0 {
		interval = time.Second
	}

	for {
		if scaler.pool.ctx.Err() != nil {
			return
		}

		time.Sleep(interval)

		read := scaler.pool.metrics.CollectReading()

		if scaler.pool.config != nil {
			for _, regulator := range scaler.pool.config.Regulators {
				regulator.Renormalize()
			}
		}

		scaler.Observe(read)
	}
}

/*
NewScaler starts the scaler loop when cfg is non-nil.
*/
func NewScaler(pool *Q[any], minWorkers, maxWorkers int, config *ScalerConfig) *Scaler {
	if config == nil {
		return nil
	}

	scaler := &Scaler{
		pool:               pool,
		minWorkers:         minWorkers,
		maxWorkers:         maxWorkers,
		targetLoad:         config.TargetLoad,
		scaleUpThreshold:   config.ScaleUpThreshold,
		scaleDownThreshold: config.ScaleDownThreshold,
		cooldown:           config.Cooldown,
		evalInterval:       config.Interval,
	}

	scaler.lastScaleDownNano.Store(time.Now().UnixNano())

	pool.scalerWG.Add(1)

	go func() {
		defer pool.scalerWG.Done()

		scaler.run()
	}()

	return scaler
}
