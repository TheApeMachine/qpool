package qpool

import (
	"math"
	"sync"
	"time"
)

/*
AdaptiveScalerRegulator implements the Regulator interface to dynamically adjust worker pool size.
It combines the functionality of the existing Scaler with additional adaptive behaviors,
similar to how an adaptive cruise control system adjusts speed based on traffic conditions.

Key features:
  - Dynamic worker pool sizing
  - Load-based scaling
  - Resource-aware adjustments
  - Performance optimization
*/
type AdaptiveScalerRegulator struct {
	mu sync.RWMutex

	pool               *Q
	minWorkers         int
	maxWorkers         int
	targetLoad         float64       // Target jobs per worker
	scaleUpThreshold   float64       // Load threshold for scaling up
	scaleDownThreshold float64       // Load threshold for scaling down
	cooldown           time.Duration // Time between scaling operations
	lastScale          time.Time     // Last scaling operation time
	metrics           *Metrics      // System metrics
}

/*
NewAdaptiveScalerRegulator creates a new adaptive scaler regulator.

Parameters:
  - pool: The worker pool to manage
  - minWorkers: Minimum number of workers
  - maxWorkers: Maximum number of workers
  - config: Scaling configuration parameters

Returns:
  - *AdaptiveScalerRegulator: A new adaptive scaler instance

Example:
    scaler := NewAdaptiveScalerRegulator(pool, 2, 10, &ScalerConfig{...})
*/
func NewAdaptiveScalerRegulator(pool *Q, minWorkers, maxWorkers int, config *ScalerConfig) *AdaptiveScalerRegulator {
	return &AdaptiveScalerRegulator{
		pool:               pool,
		minWorkers:         minWorkers,
		maxWorkers:         maxWorkers,
		targetLoad:         config.TargetLoad,
		scaleUpThreshold:   config.ScaleUpThreshold,
		scaleDownThreshold: config.ScaleDownThreshold,
		cooldown:           config.Cooldown,
		lastScale:          time.Now(),
	}
}

/*
Observe implements the Regulator interface by monitoring system metrics.
This method updates the scaler's view of system load and performance.

Parameters:
  - metrics: Current system metrics including worker and queue statistics
*/
func (as *AdaptiveScalerRegulator) Observe(metrics *Metrics) {
	as.mu.Lock()
	defer as.mu.Unlock()

	as.metrics = metrics
	as.evaluate()
}

/*
Limit implements the Regulator interface by determining if scaling operations
should be limited. Returns true during cooldown periods or at worker limits.

Returns:
  - bool: true if scaling should be limited, false if it can proceed
*/
func (as *AdaptiveScalerRegulator) Limit() bool {
	as.mu.RLock()
	defer as.mu.RUnlock()

	if as.metrics == nil {
		return false
	}

	// Limit if we're at max workers and load is high
	if as.metrics.WorkerCount >= as.maxWorkers {
		currentLoad := float64(as.metrics.JobQueueSize) / float64(as.metrics.WorkerCount)
		return currentLoad > as.scaleUpThreshold
	}

	return false
}

/*
Renormalize implements the Regulator interface by attempting to restore normal operation.
This method triggers a scaling evaluation if enough time has passed since the last scale.
*/
func (as *AdaptiveScalerRegulator) Renormalize() {
	as.mu.Lock()
	defer as.mu.Unlock()

	if time.Since(as.lastScale) >= as.cooldown {
		as.evaluate()
	}
}

// evaluate assesses current metrics and scales the worker pool accordingly
func (as *AdaptiveScalerRegulator) evaluate() {
	if as.metrics == nil || time.Since(as.lastScale) < as.cooldown {
		return
	}

	// Ensure at least one worker for load calculation
	if as.metrics.WorkerCount == 0 {
		as.metrics.WorkerCount = 1
	}

	currentLoad := float64(as.metrics.JobQueueSize) / float64(as.metrics.WorkerCount)

	switch {
	case currentLoad > as.scaleUpThreshold && as.metrics.WorkerCount < as.maxWorkers:
		needed := int(math.Ceil(float64(as.metrics.JobQueueSize) / as.targetLoad))
		toAdd := Min(as.maxWorkers-as.metrics.WorkerCount, needed)
		if toAdd > 0 {
			as.scaleUp(toAdd)
			as.lastScale = time.Now()
		}

	case currentLoad < as.scaleDownThreshold && as.metrics.WorkerCount > as.minWorkers:
		needed := Max(int(math.Ceil(float64(as.metrics.JobQueueSize)/as.targetLoad)), as.minWorkers)
		toRemove := Min(as.metrics.WorkerCount-as.minWorkers, Max(1, (as.metrics.WorkerCount-needed)/2))
		if toRemove > 0 {
			as.scaleDown(toRemove)
			as.lastScale = time.Now()
		}
	}
}

// scaleUp adds workers to the pool
func (as *AdaptiveScalerRegulator) scaleUp(count int) {
	toAdd := Min(as.maxWorkers-as.metrics.WorkerCount, Max(1, count))
	for i := 0; i < toAdd; i++ {
		as.pool.startWorker()
	}
}

// scaleDown removes workers from the pool
func (as *AdaptiveScalerRegulator) scaleDown(count int) {
	as.pool.workerMu.Lock()
	defer as.pool.workerMu.Unlock()

	for i := 0; i < count; i++ {
		if len(as.pool.workerList) == 0 {
			break
		}

		// Remove the last worker from the list
		w := as.pool.workerList[len(as.pool.workerList)-1]
		as.pool.workerList = as.pool.workerList[:len(as.pool.workerList)-1]

		// Cancel the worker's context outside the lock to avoid holding it during cleanup
		cancelFunc := w.cancel

		as.metrics.WorkerCount--

		// Release the lock before cleanup operations
		as.pool.workerMu.Unlock()

		// Cancel the worker's context
		if cancelFunc != nil {
			cancelFunc()
		}

		// Add a small delay between worker removals
		time.Sleep(time.Millisecond * 50)

		// Re-acquire the lock for the next iteration
		as.pool.workerMu.Lock()
	}
} 