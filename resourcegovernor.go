package qpool

import (
	"runtime"
	"sync"
	"time"
)

/*
ResourceGovernorRegulator implements the Regulator interface to manage system resources.
It monitors and controls resource usage (CPU, memory, etc.) to prevent system
exhaustion, similar to how a power governor prevents engine damage by limiting
power consumption under heavy load.

Key features:
  - CPU usage monitoring
  - Memory usage tracking
  - Resource thresholds
  - Adaptive limiting
*/
type ResourceGovernorRegulator struct {
	mu sync.RWMutex

	maxCPUPercent    float64       // Maximum allowed CPU usage (0.0-1.0)
	maxMemoryPercent float64       // Maximum allowed memory usage (0.0-1.0)
	checkInterval    time.Duration // How often to check resource usage
	metrics          *Metrics     // System metrics
	lastCheck        time.Time    // Last resource check time

	// Current resource usage
	currentCPU    float64
	currentMemory float64
}

/*
NewResourceGovernorRegulator creates a new resource governor regulator.

Parameters:
  - maxCPUPercent: Maximum allowed CPU usage (0.0-1.0)
  - maxMemoryPercent: Maximum allowed memory usage (0.0-1.0)
  - checkInterval: How often to check resource usage

Returns:
  - *ResourceGovernorRegulator: A new resource governor instance

Example:
    governor := NewResourceGovernorRegulator(0.8, 0.9, time.Second)
*/
func NewResourceGovernorRegulator(maxCPUPercent, maxMemoryPercent float64, checkInterval time.Duration) *ResourceGovernorRegulator {
	return &ResourceGovernorRegulator{
		maxCPUPercent:    maxCPUPercent,
		maxMemoryPercent: maxMemoryPercent,
		checkInterval:    checkInterval,
		lastCheck:        time.Now(),
	}
}

/*
Observe implements the Regulator interface by monitoring system metrics.
This method updates the governor's view of resource utilization based on
current system metrics.

Parameters:
  - metrics: Current system metrics including resource utilization data
*/
func (rg *ResourceGovernorRegulator) Observe(metrics *Metrics) {
	rg.mu.Lock()
	defer rg.mu.Unlock()

	rg.metrics = metrics
	rg.updateResourceUsage()
}

/*
Limit implements the Regulator interface by determining if resource usage
should be limited. Returns true when resource usage exceeds thresholds.

Returns:
  - bool: true if resource usage should be limited, false if it can proceed
*/
func (rg *ResourceGovernorRegulator) Limit() bool {
	rg.mu.RLock()
	defer rg.mu.RUnlock()

	// Check if either CPU or memory usage exceeds thresholds
	return rg.currentCPU >= rg.maxCPUPercent || rg.currentMemory >= rg.maxMemoryPercent
}

/*
Renormalize implements the Regulator interface by attempting to restore normal operation.
This method updates resource usage measurements and adjusts thresholds if necessary.
*/
func (rg *ResourceGovernorRegulator) Renormalize() {
	rg.mu.Lock()
	defer rg.mu.Unlock()

	// Update resource measurements
	rg.updateResourceUsage()
}

// updateResourceUsage updates current resource utilization measurements
func (rg *ResourceGovernorRegulator) updateResourceUsage() {
	if rg.metrics == nil {
		return
	}

	// Update CPU usage from metrics
	if rg.metrics.ResourceUtilization > 0 {
		rg.currentCPU = rg.metrics.ResourceUtilization
	}

	// Get current memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Calculate memory usage as percentage of total available
	totalMemory := float64(memStats.Sys)
	usedMemory := float64(memStats.Alloc)
	rg.currentMemory = usedMemory / totalMemory
}

// GetResourceUsage returns current resource utilization levels
func (rg *ResourceGovernorRegulator) GetResourceUsage() (cpu, memory float64) {
	rg.mu.RLock()
	defer rg.mu.RUnlock()
	return rg.currentCPU, rg.currentMemory
}

// GetThresholds returns the current resource usage thresholds
func (rg *ResourceGovernorRegulator) GetThresholds() (cpu, memory float64) {
	rg.mu.RLock()
	defer rg.mu.RUnlock()
	return rg.maxCPUPercent, rg.maxMemoryPercent
} 