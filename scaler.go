package qpool

import (
	"log"
	"math"
	"time"
)

// Scaler manages pool size based on current load
type Scaler struct {
	pool               *Q
	minWorkers         int
	maxWorkers         int
	targetLoad         float64
	scaleUpThreshold   float64
	scaleDownThreshold float64
	cooldown           time.Duration
	lastScale          time.Time
}

// ScalerConfig defines configuration for the Scaler
type ScalerConfig struct {
	TargetLoad         float64
	ScaleUpThreshold   float64
	ScaleDownThreshold float64
	Cooldown           time.Duration
}

// evaluate assesses the current load and scales the worker pool accordingly
func (s *Scaler) evaluate() {
	s.pool.metrics.mu.Lock()
	defer s.pool.metrics.mu.Unlock()

	if time.Since(s.lastScale) < s.cooldown {
		return
	}

	// Ensure at least one worker for load calculation
	if s.pool.metrics.WorkerCount == 0 {
		s.pool.metrics.WorkerCount = 1
	}

	currentLoad := float64(s.pool.metrics.JobQueueSize) / float64(s.pool.metrics.WorkerCount)
	log.Printf("Current load: %.2f, Workers: %d, Queue: %d",
		currentLoad, s.pool.metrics.WorkerCount, s.pool.metrics.JobQueueSize)

	switch {
	case currentLoad > s.scaleUpThreshold && s.pool.metrics.WorkerCount < s.maxWorkers:
		needed := int(math.Ceil(float64(s.pool.metrics.JobQueueSize) / s.targetLoad))
		toAdd := Min(s.maxWorkers-s.pool.metrics.WorkerCount, needed)
		if toAdd > 0 {
			s.scaleUp(toAdd)
			s.lastScale = time.Now()
		}

	case currentLoad < s.scaleDownThreshold && s.pool.metrics.WorkerCount > s.minWorkers:
		needed := Max(int(math.Ceil(float64(s.pool.metrics.JobQueueSize)/s.targetLoad)), s.minWorkers)
		toRemove := Min(s.pool.metrics.WorkerCount-s.minWorkers, Max(1, (s.pool.metrics.WorkerCount-needed)/2))
		if toRemove > 0 {
			s.scaleDown(toRemove)
			s.lastScale = time.Now()
		}
	}
}

// scaleUp adds 'count' number of workers to the pool
func (s *Scaler) scaleUp(count int) {
	toAdd := Min(s.maxWorkers-s.pool.metrics.WorkerCount, Max(1, count))

	for i := 0; i < toAdd; i++ {
		s.pool.startWorker()
	}
	log.Printf("Scaled up by %d workers, total workers: %d", toAdd, s.pool.metrics.WorkerCount)
}

// scaleDown removes 'count' number of workers from the pool
func (s *Scaler) scaleDown(count int) {
	s.pool.workerMu.Lock()
	defer s.pool.workerMu.Unlock()

	for i := 0; i < count; i++ {
		if len(s.pool.workerList) == 0 {
			break
		}

		// Remove the last worker from the list
		w := s.pool.workerList[len(s.pool.workerList)-1]
		s.pool.workerList = s.pool.workerList[:len(s.pool.workerList)-1]

		// Cancel the worker's context outside the lock to avoid holding it during cleanup
		cancelFunc := w.cancel

		s.pool.metrics.WorkerCount--

		// Release the lock before cleanup operations
		s.pool.workerMu.Unlock()

		// Cancel the worker's context
		if cancelFunc != nil {
			cancelFunc()
		}

		log.Printf("Scaled down worker, total workers: %d", s.pool.metrics.WorkerCount)

		// Add a small delay between worker removals
		time.Sleep(time.Millisecond * 50)

		// Re-acquire the lock for the next iteration
		s.pool.workerMu.Lock()
	}
}

// run starts the scaler's evaluation loop
func (s *Scaler) run() {
	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	for {
		select {
		case <-s.pool.ctx.Done():
			return
		case <-ticker.C:
			s.evaluate()
		}
	}
}

// NewScaler initializes and starts a new Scaler
func NewScaler(q *Q, minWorkers, maxWorkers int, config *ScalerConfig) *Scaler {
	scaler := &Scaler{
		pool:               q,
		minWorkers:         minWorkers,
		maxWorkers:         maxWorkers,
		targetLoad:         config.TargetLoad,
		scaleUpThreshold:   config.ScaleUpThreshold,
		scaleDownThreshold: config.ScaleDownThreshold,
		cooldown:           config.Cooldown,
		lastScale:          time.Now(),
	}

	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		scaler.run()
	}()

	return scaler
}
