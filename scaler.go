package qpool

import (
	"context"
	"math"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
)

/*
Scaler periodically evaluates queue depth and scales worker count on load.
It does not reject Schedule; back-pressure is the disruptor queue and optional Regulators.
*/
type Scaler struct {
	ctx                context.Context
	cancel             context.CancelFunc
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
	// Interval is the ticker period for CollectReading and
	// evaluate. Zero defaults to one second inside NewScaler.
	Interval time.Duration
}

func (config *ScalerConfig) jobQueueCapacity(maxWorkers int) int {
	if config == nil {
		return int(math.Max(1, float64(maxWorkers)))
	}

	threshold := math.Max(1, config.ScaleUpThreshold)
	return int(math.Ceil(float64(maxWorkers) * threshold))
}

/*
Observe refreshes metrics and may scale the worker count up immediately.
*/
func (scaler *Scaler) Observe(reading MetricReading) {
	if scaler == nil {
		return
	}

	scaler.reading.Store(&reading)
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
		reading := scaler.pool.metrics.CollectReading()
		scaler.reading.Store(&reading)
		read = &reading
	}

	workers := max(1, read.WorkerCount)
	targetLoad := max(1, scaler.targetLoad)

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
		needed := max(int(math.Ceil(
			float64(read.JobQueueSize)/targetLoad,
		)), scaler.minWorkers)

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
	for range min(scaler.maxWorkers-int(
		scaler.pool.metrics.workerCount.Load(),
	), count) {
		scaler.pool.startWorker()
	}
}

func (scaler *Scaler) noteScaleUp(delta int) {
	now := time.Now()
	scaler.pool.metrics.NoteLastScale(now)

	artifact := datura.Acquire("qpool", datura.Artifact_Type_json)
	artifact.SetRole("op")
	artifact.SetPayload([]byte("scaled worker pool up"))
	artifact.SetTimestamp(now.UnixNano())
	artifact.SetScope("debug")
	artifact.Poke("delta", strconv.Itoa(delta))
	artifact.Poke("workers", strconv.FormatInt(scaler.pool.metrics.workerCount.Load(), 10))
	scaler.pool.publishTelemetry(artifact)
}

func (scaler *Scaler) noteScaleDown(delta int) {
	now := time.Now()
	scaler.lastScaleDownNano.Store(now.UnixNano())
	scaler.pool.metrics.NoteLastScale(now)

	artifact := datura.Acquire("qpool", datura.Artifact_Type_json)
	artifact.SetRole("op")
	artifact.SetPayload([]byte("scaled worker pool down"))
	artifact.SetTimestamp(now.UnixNano())
	artifact.SetScope("debug")
	artifact.Poke("delta", strconv.Itoa(delta))
	artifact.Poke("workers", strconv.FormatInt(scaler.pool.metrics.workerCount.Load(), 10))
	scaler.pool.publishTelemetry(artifact)
}

/*
NewScaler starts the scaler loop when cfg is non-nil.
*/
func NewScaler(
	ctx context.Context,
	pool *Q[any],
	minWorkers, maxWorkers int,
	config *ScalerConfig,
) *Scaler {
	ctx, cancel := context.WithCancel(ctx)

	scaler := &Scaler{
		ctx:                ctx,
		cancel:             cancel,
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
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			time.Sleep(max(time.Second, scaler.evalInterval))

			if scaler.pool.config != nil {
				for _, regulator := range scaler.pool.config.Regulators {
					regulator.Renormalize()
				}
			}

			scaler.Observe(scaler.pool.metrics.CollectReading())
		}
	}()

	return scaler
}
