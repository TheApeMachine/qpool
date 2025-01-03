package qpool

import (
	"errors"
	"log"
	"sync"
	"time"
)

// ErrNoAvailableWorkers is returned when no workers are available to process a job
var ErrNoAvailableWorkers = errors.New("no workers available to process job")

/*
LoadBalancer implements the Regulator interface to provide intelligent work distribution.
It ensures even distribution of work across workers while considering system metrics
like worker load, processing speed, and resource utilization.

Like a traffic controller directing vehicles to different lanes based on congestion,
the load balancer directs work to workers based on their current capacity and
performance characteristics.

Key features:
  - Even work distribution
  - Metric-based routing decisions
  - Automatic worker selection
  - Adaptive load management
*/
type LoadBalancer struct {
	mu sync.RWMutex

	workerLoads    map[int]float64 // Current load per worker
	workerLatency  map[int]time.Duration // Average processing time per worker
	workerCapacity map[int]int     // Maximum concurrent jobs per worker
	activeWorkers  int            // Number of available workers
	metrics        *Metrics       // System metrics for adaptive behavior
}

/*
NewLoadBalancer creates a new load balancer regulator with specified parameters.

Parameters:
  - workerCount: Initial number of workers to balance between
  - workerCapacity: Maximum concurrent jobs per worker

Returns:
  - *LoadBalancer: A new load balancer instance

Example:
    balancer := NewLoadBalancer(5, 10) // 5 workers, 10 jobs each
*/
func NewLoadBalancer(workerCount, workerCapacity int) *LoadBalancer {
	lb := &LoadBalancer{
		workerLoads:    make(map[int]float64),
		workerLatency:  make(map[int]time.Duration),
		workerCapacity: make(map[int]int),
		activeWorkers:  workerCount,
	}

	// Initialize worker capacities
	for i := 0; i < workerCount; i++ {
		lb.workerCapacity[i] = workerCapacity
		lb.workerLoads[i] = 0.0
		lb.workerLatency[i] = 0
	}

	return lb
}

/*
Observe implements the Regulator interface by monitoring system metrics.
This method updates the load balancer's view of worker performance and system state,
allowing it to make informed routing decisions.

Parameters:
  - metrics: Current system metrics including worker performance data
*/
func (lb *LoadBalancer) Observe(metrics *Metrics) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.metrics = metrics
	lb.updateWorkerStats()
}

/*
Limit implements the Regulator interface by determining if work should be limited.
Returns true if all workers are at capacity and no more work should be accepted.

Returns:
  - bool: true if work should be limited, false if it can proceed

Thread-safety: This method is thread-safe through mutex protection.
*/
func (lb *LoadBalancer) Limit() bool {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	// Check if any worker has capacity
	for i := 0; i < lb.activeWorkers; i++ {
		if lb.workerLoads[i] < float64(lb.workerCapacity[i]) {
			return false
		}
	}
	return true
}

/*
Renormalize implements the Regulator interface by attempting to restore normal operation.
This method resets worker statistics and redistributes load if necessary.
*/
func (lb *LoadBalancer) Renormalize() {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Reset worker loads if they seem incorrect
	for i := 0; i < lb.activeWorkers; i++ {
		if lb.workerLoads[i] > float64(lb.workerCapacity[i]) {
			lb.workerLoads[i] = float64(lb.workerCapacity[i])
		}
	}
}

/*
SelectWorker chooses the most appropriate worker for the next job based on
current load distribution and worker performance metrics.

Returns:
  - int: The selected worker ID
  - error: Error if no suitable worker is available
*/
func (lb *LoadBalancer) SelectWorker() (int, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	selectedWorker := -1

	for i := 0; i < lb.activeWorkers; i++ {
		// Skip workers at capacity
		if lb.workerLoads[i] >= float64(lb.workerCapacity[i]) {
			log.Printf("Worker %d at capacity: load=%v, capacity=%v", i, lb.workerLoads[i], lb.workerCapacity[i])
			continue
		}

		// If no worker selected yet, select this one
		if selectedWorker == -1 {
			log.Printf("First worker %d: load=%v, latency=%v", i, lb.workerLoads[i], lb.workerLatency[i])
			selectedWorker = i
			continue
		}

		log.Printf("Comparing worker %d (load=%v, latency=%v) with selected worker %d (load=%v, latency=%v)",
			i, lb.workerLoads[i], lb.workerLatency[i],
			selectedWorker, lb.workerLoads[selectedWorker], lb.workerLatency[selectedWorker])

		// Compare loads first
		if lb.workerLoads[i] < lb.workerLoads[selectedWorker] {
			log.Printf("Selected worker %d due to lower load", i)
			selectedWorker = i
		} else if lb.workerLoads[i] == lb.workerLoads[selectedWorker] {
			// If loads are equal, compare latencies
			// Only consider latency if both workers have non-zero latency
			if lb.workerLatency[selectedWorker] == 0 || 
				(lb.workerLatency[i] > 0 && lb.workerLatency[i] < lb.workerLatency[selectedWorker]) {
				log.Printf("Selected worker %d due to better latency", i)
				selectedWorker = i
			}
		}
	}

	if selectedWorker == -1 {
		return -1, ErrNoAvailableWorkers
	}

	log.Printf("Final selection: worker %d", selectedWorker)
	return selectedWorker, nil
}

/*
RecordJobStart updates worker statistics when a job starts processing.
This helps maintain accurate load information for future routing decisions.

Parameters:
  - workerID: The ID of the worker that started the job
*/
func (lb *LoadBalancer) RecordJobStart(workerID int) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if workerID >= 0 && workerID < lb.activeWorkers {
		lb.workerLoads[workerID]++
	}
}

/*
RecordJobComplete updates worker statistics when a job completes processing.
This helps maintain accurate load and performance information.

Parameters:
  - workerID: The ID of the worker that completed the job
  - duration: How long the job took to process
*/
func (lb *LoadBalancer) RecordJobComplete(workerID int, duration time.Duration) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if workerID >= 0 && workerID < lb.activeWorkers {
		lb.workerLoads[workerID]--
		if lb.workerLoads[workerID] < 0 {
			lb.workerLoads[workerID] = 0
		}

		// Update moving average of worker latency
		if lb.workerLatency[workerID] == 0 {
			lb.workerLatency[workerID] = duration
		} else {
			lb.workerLatency[workerID] = (lb.workerLatency[workerID] * 4 + duration) / 5
		}
	}
}

// updateWorkerStats updates internal statistics based on observed metrics
func (lb *LoadBalancer) updateWorkerStats() {
	if lb.metrics == nil {
		return
	}

	// Update active workers count if it has changed
	if lb.metrics.WorkerCount != lb.activeWorkers {
		// Adjust capacity maps for new worker count
		newCount := lb.metrics.WorkerCount
		if newCount > lb.activeWorkers {
			// Initialize new workers
			for i := lb.activeWorkers; i < newCount; i++ {
				lb.workerCapacity[i] = lb.workerCapacity[0] // Use same capacity as first worker
				lb.workerLoads[i] = 0.0
				lb.workerLatency[i] = 0
			}
		}
		lb.activeWorkers = newCount
	}
} 