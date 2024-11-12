package qpool

import (
	"log"
	"math"
	"time"
)

// Scaler implementation
func (s *Scaler) evaluate() {
	s.pool.metrics.mu.Lock()
	defer s.pool.metrics.mu.Unlock()

	if time.Since(s.pool.metrics.LastScale) < s.cooldown {
		return
	}

	currentLoad := float64(s.pool.metrics.JobQueueSize) / float64(s.pool.metrics.WorkerCount)

	switch {
	case currentLoad > s.scaleUpThreshold && s.pool.metrics.WorkerCount < s.maxWorkers:
		needed := int(math.Ceil(float64(s.pool.metrics.JobQueueSize) / s.targetLoad))
		toAdd := min(needed-s.pool.metrics.WorkerCount, s.maxWorkers-s.pool.metrics.WorkerCount)
		s.scaleUp(toAdd)

	case currentLoad < s.scaleDownThreshold && s.pool.metrics.WorkerCount > s.minWorkers:
		needed := max(int(math.Ceil(float64(s.pool.metrics.JobQueueSize)/s.targetLoad)), s.minWorkers)
		toRemove := s.pool.metrics.WorkerCount - needed
		s.scaleDown(toRemove)
	}

	s.pool.metrics.LastScale = time.Now()
}

func (s *Scaler) scaleUp(count int) {
	for i := 0; i < count; i++ {
		s.pool.startWorker()
		s.pool.metrics.mu.Lock()
		s.pool.metrics.WorkerCount++
		s.pool.metrics.mu.Unlock()
		log.Printf("Scaled up worker, total workers: %d", s.pool.metrics.WorkerCount)
	}
}

func (s *Scaler) scaleDown(count int) {
	s.pool.workerMu.Lock()
	defer s.pool.workerMu.Unlock()

	for i := 0; i < count && len(s.pool.workerList) > 0; i++ {
		// Remove worker from the list
		w := s.pool.workerList[len(s.pool.workerList)-1]
		s.pool.workerList = s.pool.workerList[:len(s.pool.workerList)-1]

		// Cancel the worker's context
		w.cancel()

		s.pool.metrics.mu.Lock()
		s.pool.metrics.WorkerCount--
		s.pool.metrics.mu.Unlock()

		log.Printf("Scaled down worker, total workers: %d", s.pool.metrics.WorkerCount)
	}
}
