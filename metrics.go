package qpool

import (
	"sort"
	"sync"
	"time"
)

type timeWindow struct {
	duration time.Duration
	count    int
}

type Metrics struct {
	mu            sync.RWMutex
	WorkerCount   int
	JobQueueSize  int
	ActiveWorkers int
	LastScale     time.Time
	ErrorRates    map[string]float64
	TotalJobTime  time.Duration
	JobCount      int64

	// Additional suggested metrics
	AverageJobLatency    time.Duration
	P95JobLatency        time.Duration
	P99JobLatency        time.Duration
	JobSuccessRate       float64
	CircuitBreakerStates map[string]CircuitState
	QueueWaitTime        time.Duration
	ResourceUtilization  float64

	// Rate limiting metrics
	RateLimitHits int64
	ThrottledJobs int64

	// Add fields for percentile calculation
	latencyWindows []timeWindow
	windowSize     int
}

func newMetrics() *Metrics {
	return &Metrics{
		ErrorRates:     make(map[string]float64),
		latencyWindows: make([]timeWindow, 0, 1000), // Store last 1000 measurements
		windowSize:     1000,
	}
}

// Add prometheus-style metrics collection
func (m *Metrics) recordJobExecution(startTime time.Time, success bool) {
	duration := time.Since(startTime)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalJobTime += duration
	m.JobCount++

	// Update success rate
	if success {
		m.JobSuccessRate = float64(m.JobCount-int64(len(m.ErrorRates))) / float64(m.JobCount)
	}

	// Update latency percentiles (simplified implementation)
	// In production, consider using a proper percentile calculation library
	m.updateLatencyPercentiles(duration)
}

// Add updateLatencyPercentiles method
func (m *Metrics) updateLatencyPercentiles(duration time.Duration) {
	// Update average using existing calculation
	m.AverageJobLatency = (m.AverageJobLatency*time.Duration(m.JobCount-1) + duration) / time.Duration(m.JobCount)

	// Add new duration to sliding window
	m.latencyWindows = append(m.latencyWindows, timeWindow{
		duration: duration,
		count:    1,
	})

	// Remove oldest entries if we exceed window size
	if len(m.latencyWindows) > m.windowSize {
		m.latencyWindows = m.latencyWindows[1:]
	}

	// Sort durations for percentile calculation
	sorted := make([]time.Duration, 0, len(m.latencyWindows))
	for _, w := range m.latencyWindows {
		for i := 0; i < w.count; i++ {
			sorted = append(sorted, w.duration)
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	// Calculate percentiles
	if len(sorted) > 0 {
		p95Index := int(float64(len(sorted)) * 0.95)
		p99Index := int(float64(len(sorted)) * 0.99)

		// Ensure indices are within bounds
		if p95Index >= len(sorted) {
			p95Index = len(sorted) - 1
		}
		if p99Index >= len(sorted) {
			p99Index = len(sorted) - 1
		}

		m.P95JobLatency = sorted[p95Index]
		m.P99JobLatency = sorted[p99Index]
	}
}

// Add metrics export functionality
func (m *Metrics) ExportMetrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"worker_count":         m.WorkerCount,
		"queue_size":           m.JobQueueSize,
		"success_rate":         m.JobSuccessRate,
		"avg_latency":          m.AverageJobLatency.Milliseconds(),
		"p95_latency":          m.P95JobLatency.Milliseconds(),
		"p99_latency":          m.P99JobLatency.Milliseconds(),
		"resource_utilization": m.ResourceUtilization,
	}
}
