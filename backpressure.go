package qpool

import (
	"sync"
	"time"
)

// MinFloat returns the smaller of two float64 values
func MinFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// MaxFloat returns the larger of two float64 values
func MaxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

/*
BackPressureRegulator implements the Regulator interface to prevent system overload.
It monitors queue depth and processing times to regulate job intake, similar to how
pressure regulators in plumbing systems prevent pipe damage by limiting flow when
pressure builds up.

Key features:
  - Queue depth monitoring
  - Processing time tracking
  - Adaptive pressure thresholds
  - Gradual flow control
*/
type BackPressureRegulator struct {
	mu sync.RWMutex

	maxQueueSize      int           // Maximum allowed queue size
	targetProcessTime time.Duration // Target job processing time
	pressureWindow    time.Duration // Time window for pressure calculation
	currentPressure   float64       // Current system pressure (0.0-1.0)
	metrics          *Metrics      // System metrics
	lastCheck        time.Time     // Last pressure check time
}

/*
NewBackPressureRegulator creates a new back pressure regulator.

Parameters:
  - maxQueueSize: Maximum allowed queue size before applying back pressure
  - targetProcessTime: Target job processing time
  - pressureWindow: Time window for pressure calculations

Returns:
  - *BackPressureRegulator: A new back pressure regulator instance

Example:
    regulator := NewBackPressureRegulator(1000, time.Second, time.Minute)
*/
func NewBackPressureRegulator(maxQueueSize int, targetProcessTime, pressureWindow time.Duration) *BackPressureRegulator {
	return &BackPressureRegulator{
		maxQueueSize:      maxQueueSize,
		targetProcessTime: targetProcessTime,
		pressureWindow:    pressureWindow,
		currentPressure:   0.0,
		lastCheck:         time.Now(),
	}
}

/*
Observe implements the Regulator interface by monitoring system metrics.
This method updates the regulator's view of system pressure based on queue size
and processing times.

Parameters:
  - metrics: Current system metrics including queue and timing data
*/
func (bp *BackPressureRegulator) Observe(metrics *Metrics) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	bp.metrics = metrics
	bp.updatePressure()
}

/*
Limit implements the Regulator interface by determining if job intake should be limited.
Returns true when system pressure exceeds acceptable levels.

Returns:
  - bool: true if job intake should be limited, false if it can proceed
*/
func (bp *BackPressureRegulator) Limit() bool {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	// Apply back pressure based on current pressure level
	return bp.currentPressure >= 0.8 // Limit at 80% pressure
}

/*
Renormalize implements the Regulator interface by attempting to restore normal operation.
This method gradually reduces system pressure if conditions allow.
*/
func (bp *BackPressureRegulator) Renormalize() {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// Gradually reduce pressure if queue size and processing times are improving
	if bp.metrics != nil && 
	   bp.metrics.JobQueueSize < bp.maxQueueSize/2 && 
	   bp.metrics.AverageJobLatency < bp.targetProcessTime {
		bp.currentPressure = MaxFloat(0.0, bp.currentPressure-0.1)
	}
}

// updatePressure calculates current system pressure based on metrics
func (bp *BackPressureRegulator) updatePressure() {
	if bp.metrics == nil {
		return
	}

	// Calculate queue pressure (0.0-1.0)
	queuePressure := float64(bp.metrics.JobQueueSize) / float64(bp.maxQueueSize)

	// Calculate timing pressure (0.0-1.0)
	timingPressure := 0.0
	if bp.metrics.AverageJobLatency > 0 {
		timingPressure = float64(bp.metrics.AverageJobLatency) / float64(bp.targetProcessTime)
	}

	// Combine pressures with weights
	bp.currentPressure = (queuePressure*0.6 + timingPressure*0.4)

	// Ensure pressure stays in valid range
	bp.currentPressure = MinFloat(1.0, MaxFloat(0.0, bp.currentPressure))
}

// GetPressure returns the current system pressure level
func (bp *BackPressureRegulator) GetPressure() float64 {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.currentPressure
} 