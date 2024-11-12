package qpool

import (
	"log"
	"math"
	"time"
)

// Scaler manages pool size
type Scaler struct {
	pool               *Q
	minWorkers         int
	maxWorkers         int
	targetLoad         float64
	scaleUpThreshold   float64
	scaleDownThreshold float64
	cooldown           time.Duration
}

// Scaler implementation
func (s *Scaler) evaluate() {
	s.pool.metrics.mu.Lock()
	defer s.pool.metrics.mu.Unlock()

	if time.Since(s.pool.metrics.LastScale) < s.cooldown {
		return
	}

	// Ensure we have at least one worker for load calculation
	if s.pool.metrics.WorkerCount == 0 {
		s.pool.metrics.WorkerCount = 1
	}

	currentLoad := float64(s.pool.metrics.JobQueueSize) / float64(s.pool.metrics.WorkerCount)

	switch {
	case currentLoad > s.scaleUpThreshold && s.pool.metrics.WorkerCount < s.maxWorkers:
		needed := int(math.Ceil(float64(s.pool.metrics.JobQueueSize) / s.targetLoad))
		// More gradual scaling: Add 25% of needed workers or 1, whichever is larger
		toAdd := min(
			s.maxWorkers-s.pool.metrics.WorkerCount,
			max(1, (needed-s.pool.metrics.WorkerCount)/4),
		)
		if toAdd > 0 {
			s.scaleUp(toAdd)
		}

	case currentLoad < s.scaleDownThreshold && s.pool.metrics.WorkerCount > s.minWorkers:
		needed := max(int(math.Ceil(float64(s.pool.metrics.JobQueueSize)/s.targetLoad)), s.minWorkers)
		// More gradual scaling: Remove 25% of excess workers or 1, whichever is larger
		toRemove := max(1, (s.pool.metrics.WorkerCount-needed)/4)
		if toRemove > 0 {
			s.scaleDown(toRemove)
		}
	}

	s.pool.metrics.LastScale = time.Now()
}

// minDuration returns the smaller of two time.Duration values
func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func (s *Scaler) scaleUp(count int) {
	// Add exponential backoff for rapid scaling
	backoff := time.Millisecond * 50
	for i := 0; i < count; i++ {
		s.pool.startWorker()
		s.pool.metrics.mu.Lock()
		s.pool.metrics.WorkerCount++
		s.pool.metrics.mu.Unlock()
		log.Printf("Scaled up worker, total workers: %d", s.pool.metrics.WorkerCount)

		// Add small delay between worker creation to prevent resource spikes
		time.Sleep(backoff)
		backoff = minDuration(backoff*2, time.Second) // Cap backoff at 1 second
	}
}

func (s *Scaler) scaleDown(count int) {
	s.pool.workerMu.Lock()
	defer s.pool.workerMu.Unlock()

	// Scale down gradually with delay between each worker removal
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

		// Add small delay between worker removal
		time.Sleep(time.Millisecond * 50)
	}
}
