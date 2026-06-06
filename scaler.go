package qpool

import (
	"math"
	"sync/atomic"
	"time"

	"github.com/phuslu/log"
)

/*
Scaler periodically evaluates queue depth and implements Regulator for schedule-time scaling.
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
	lastScaleUnixNano  atomic.Int64
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
Observe refreshes metrics and may scale the worker count.
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
Limit rejects schedules when the pool is saturated at max workers.
*/
func (scaler *Scaler) Limit() bool {
	if scaler == nil {
		return false
	}

	read := scaler.reading.Load()

	if read == nil {
		return false
	}

	if read.WorkerCount < scaler.maxWorkers {
		return false
	}

	load := float64(read.JobQueueSize) / float64(max(1, read.WorkerCount))

	return load > scaler.scaleUpThreshold
}

/*
Renormalize retries scaling evaluation after the cooldown window.
*/
func (scaler *Scaler) Renormalize() {
	if scaler == nil {
		return
	}

	last := scaler.lastScaleUnixNano.Load()

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

	last := time.Unix(0, scaler.lastScaleUnixNano.Load())

	if time.Since(last) < scaler.cooldown {
		return
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

	switch {
	case currentLoad > scaler.scaleUpThreshold && read.WorkerCount < scaler.maxWorkers:
		needed := int(math.Ceil(float64(read.JobQueueSize) / targetLoad))
		delta := max(0, needed-read.WorkerCount)
		toAdd := min(scaler.maxWorkers-read.WorkerCount, delta)

		if toAdd > 0 {
			scaler.scaleUp(toAdd)
			scaler.noteScale(toAdd, "scale-up", "scaled worker pool up")
		}

	case currentLoad < scaler.scaleDownThreshold && read.WorkerCount > scaler.minWorkers:
		needed := max(int(math.Ceil(float64(read.JobQueueSize)/targetLoad)), scaler.minWorkers)

		toRemove := min(
			read.WorkerCount-scaler.minWorkers,
			max(1, (read.WorkerCount-needed)/2),
		)

		if toRemove > 0 {
			scaler.pool.scaleDownWorkers(toRemove)
			scaler.noteScale(toRemove, "scale-down", "scaled worker pool down")
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

func (scaler *Scaler) noteScale(delta int, op, message string) {
	now := time.Now()

	scaler.lastScaleUnixNano.Store(now.UnixNano())
	scaler.pool.metrics.NoteLastScale(now)

	scaler.pool.publishTelemetry(Event{
		Component: "qpool",
		Op:        op,
		Message:   message,
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

	scaler.lastScaleUnixNano.Store(time.Now().UnixNano())

	pool.scalerWG.Add(1)

	go func() {
		defer pool.scalerWG.Done()

		scaler.run()
	}()

	return scaler
}
