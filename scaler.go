package qpool

import (
	"math"
	"time"

	"github.com/phuslu/log"
)

/*
Scaler periodically evaluates queue depth using atomic metrics and adjusts worker count.
*/
type Scaler struct {
	pool               *Q
	minWorkers         int
	maxWorkers         int
	targetLoad         float64
	scaleUpThreshold   float64
	scaleDownThreshold float64
	cooldown           time.Duration
	evalInterval       time.Duration
	lastScale          time.Time
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

func (s *Scaler) evaluate(read *MetricReading) {
	if read == nil {
		read = s.pool.metrics.CollectReading()
	}

	if time.Since(s.lastScale) < s.cooldown {
		return
	}

	workers := read.WorkerCount

	if workers <= 0 {
		workers = 1
	}

	currentLoad := float64(read.JobQueueSize) / float64(workers)

	tl := s.targetLoad

	if tl <= 0 {
		tl = 1
	}

	switch {
	case currentLoad > s.scaleUpThreshold && read.WorkerCount < s.maxWorkers:
		needed := int(math.Ceil(float64(read.JobQueueSize) / tl))
		delta := max(0, needed-read.WorkerCount)
		toAdd := min(s.maxWorkers-read.WorkerCount, delta)

		if toAdd > 0 {
			for range toAdd {
				s.pool.startWorker()
			}

			s.lastScale = time.Now()
			s.pool.metrics.NoteLastScale(s.lastScale)

			s.pool.publishTelemetry(Event{
				Component: "qpool",
				Op:        "scale-up",
				Message:   "scaled worker pool up",
				Time:      time.Now(),
				Level:     log.DebugLevel,
				Fields: []Field{
					{Key: "delta", Value: toAdd},
					{Key: "workers", Value: s.pool.metrics.workerCount.Load()},
				},
			})
		}

	case currentLoad < s.scaleDownThreshold && read.WorkerCount > s.minWorkers:
		needed := max(int(math.Ceil(float64(read.JobQueueSize)/tl)), s.minWorkers)

		toRemove := min(read.WorkerCount-s.minWorkers, max(1, (read.WorkerCount-needed)/2))

		if toRemove > 0 {
			s.pool.scaleDownWorkers(toRemove)

			s.lastScale = time.Now()
			s.pool.metrics.NoteLastScale(s.lastScale)

			s.pool.publishTelemetry(Event{
				Component: "qpool",
				Op:        "scale-down",
				Message:   "scaled worker pool down",
				Time:      time.Now(),
				Level:     log.DebugLevel,
				Fields: []Field{
					{Key: "delta", Value: toRemove},
					{Key: "workers", Value: s.pool.metrics.workerCount.Load()},
				},
			})
		}
	}
}

func (s *Scaler) run() {
	interval := s.evalInterval

	if interval <= 0 {
		interval = time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.pool.ctx.Done():
			return

		case <-ticker.C:
			read := s.pool.metrics.CollectReading()

			if s.pool.config != nil {
				for _, reg := range s.pool.config.Regulators {
					reg.Renormalize()
				}
			}

			s.evaluate(read)
		}
	}
}

/*
NewScaler starts the scaler loop when cfg is non-nil.
*/
func NewScaler(q *Q, minWorkers, maxWorkers int, config *ScalerConfig) *Scaler {
	if config == nil {
		return nil
	}

	scaler := &Scaler{
		pool:               q,
		minWorkers:         minWorkers,
		maxWorkers:         maxWorkers,
		targetLoad:         config.TargetLoad,
		scaleUpThreshold:   config.ScaleUpThreshold,
		scaleDownThreshold: config.ScaleDownThreshold,
		cooldown:           config.Cooldown,
		evalInterval:       config.Interval,
		lastScale:          time.Now(),
	}

	q.wg.Add(1)

	go func() {
		defer q.wg.Done()

		scaler.run()
	}()

	return scaler
}
