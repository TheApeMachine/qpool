package qpool

import (
	"math"
	"sort"
	"sync"
	"time"
)

// tDigestCentroid represents a centroid in the t-digest
type tDigestCentroid struct {
	mean  float64
	count int64
}

type Metrics struct {
	mu                   sync.RWMutex
	WorkerCount          int
	JobQueueSize         int
	ActiveWorkers        int
	LastScale            time.Time
	ErrorRates           map[string]float64
	TotalJobTime         time.Duration
	JobCount             int64
	CircuitBreakerStates map[string]CircuitState

	// Additional suggested metrics
	AverageJobLatency   time.Duration
	P95JobLatency       time.Duration
	P99JobLatency       time.Duration
	JobSuccessRate      float64
	QueueWaitTime       time.Duration
	ResourceUtilization float64

	// Rate limiting metrics
	RateLimitHits int64
	ThrottledJobs int64

	// t-digest fields for percentile calculation
	centroids    []tDigestCentroid
	compression  float64
	totalWeight  int64
	maxCentroids int

	// SchedulingFailures field to track scheduling timeouts
	SchedulingFailures int64

	// Additional metrics
	FailureCount int64
}

func newMetrics() *Metrics {
	return &Metrics{
		ErrorRates:           make(map[string]float64),
		CircuitBreakerStates: make(map[string]CircuitState),
		SchedulingFailures:   0,
		compression:          100,
		maxCentroids:         100,
		centroids:            make([]tDigestCentroid, 0, 100),
		totalWeight:          0,
		JobSuccessRate:       1.0,
	}
}

// Add prometheus-style metrics collection
func (m *Metrics) recordJobExecution(startTime time.Time, success bool) {
	m.mu.RLock()
	oldTime := m.TotalJobTime
	m.mu.RUnlock()

	duration := time.Since(startTime)

	m.mu.Lock()
	m.TotalJobTime = oldTime + duration
	m.JobCount++
	if success {
		m.JobSuccessRate = float64(m.JobCount-m.FailureCount) / float64(m.JobCount)
	}
	m.mu.Unlock()

	// Update latency percentiles in a separate lock to reduce contention
	m.updateLatencyPercentiles(duration)
}

// Add updateLatencyPercentiles method
func (m *Metrics) updateLatencyPercentiles(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update average using existing calculation
	m.AverageJobLatency = (m.AverageJobLatency*time.Duration(m.JobCount-1) + duration) / time.Duration(m.JobCount)

	// Convert duration to float64 milliseconds for t-digest
	value := float64(duration.Milliseconds())

	// Find the closest centroid or create a new one
	inserted := false
	m.totalWeight++

	if len(m.centroids) == 0 {
		m.centroids = append(m.centroids, tDigestCentroid{mean: value, count: 1})
		return
	}

	// Find insertion point
	idx := sort.Search(len(m.centroids), func(i int) bool {
		return m.centroids[i].mean >= value
	})

	// Calculate maximum weight for this point
	q := m.calculateQuantile(value)
	maxWeight := int64(4 * m.compression * math.Min(q, 1-q))

	// Try to merge with existing centroid
	if idx < len(m.centroids) && m.centroids[idx].count < maxWeight {
		c := &m.centroids[idx]
		c.mean = (c.mean*float64(c.count) + value) / float64(c.count+1)
		c.count++
		inserted = true
	} else if idx > 0 && m.centroids[idx-1].count < maxWeight {
		c := &m.centroids[idx-1]
		c.mean = (c.mean*float64(c.count) + value) / float64(c.count+1)
		c.count++
		inserted = true
	}

	// If we couldn't merge, insert new centroid
	if !inserted {
		newCentroid := tDigestCentroid{mean: value, count: 1}
		m.centroids = append(m.centroids, tDigestCentroid{})
		copy(m.centroids[idx+1:], m.centroids[idx:])
		m.centroids[idx] = newCentroid
	}

	// Compress if we have too many centroids
	if len(m.centroids) > m.maxCentroids {
		m.compress()
	}

	// Update P95 and P99
	m.P95JobLatency = time.Duration(m.estimatePercentile(0.95)) * time.Millisecond
	m.P99JobLatency = time.Duration(m.estimatePercentile(0.99)) * time.Millisecond
}

func (m *Metrics) calculateQuantile(value float64) float64 {
	rank := 0.0
	for _, c := range m.centroids {
		if c.mean < value {
			rank += float64(c.count)
		}
	}
	return rank / float64(m.totalWeight)
}

func (m *Metrics) estimatePercentile(p float64) float64 {
	if len(m.centroids) == 0 {
		return 0
	}

	targetRank := p * float64(m.totalWeight)
	cumulative := 0.0

	for i, c := range m.centroids {
		cumulative += float64(c.count)
		if cumulative >= targetRank {
			// Linear interpolation between centroids
			if i > 0 {
				prev := m.centroids[i-1]
				prevCumulative := cumulative - float64(c.count)
				t := (targetRank - prevCumulative) / float64(c.count)
				return prev.mean + t*(c.mean-prev.mean)
			}
			return c.mean
		}
	}
	return m.centroids[len(m.centroids)-1].mean
}

func (m *Metrics) compress() {
	if len(m.centroids) <= 1 {
		return
	}

	// Sort centroids by mean if needed
	sort.Slice(m.centroids, func(i, j int) bool {
		return m.centroids[i].mean < m.centroids[j].mean
	})

	// Merge adjacent centroids while respecting size constraints
	newCentroids := make([]tDigestCentroid, 0, m.maxCentroids)
	current := m.centroids[0]

	for i := 1; i < len(m.centroids); i++ {
		if current.count+m.centroids[i].count <= int64(m.compression) {
			// Merge centroids
			totalCount := current.count + m.centroids[i].count
			current.mean = (current.mean*float64(current.count) +
				m.centroids[i].mean*float64(m.centroids[i].count)) /
				float64(totalCount)
			current.count = totalCount
		} else {
			newCentroids = append(newCentroids, current)
			current = m.centroids[i]
		}
	}
	newCentroids = append(newCentroids, current)
	m.centroids = newCentroids
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

func (m *Metrics) RecordJobSuccess(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.JobCount++
	m.TotalJobTime += latency
	m.AverageJobLatency = time.Duration(int64(m.TotalJobTime) / m.JobCount)
	// Update t-digest for percentiles
	m.updateLatencyMetrics(latency)
	m.JobSuccessRate = float64(m.JobCount-m.FailureCount) / float64(m.JobCount)
}

// RecordJobFailure records the failure of a job and updates metrics
func (m *Metrics) RecordJobFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FailureCount++
	m.JobSuccessRate = float64(m.JobCount-m.FailureCount) / float64(m.JobCount)
}

// updateLatencyMetrics updates latency percentiles
func (m *Metrics) updateLatencyMetrics(latency time.Duration) {
	// Simple implementation: update P95 and P99 if current latency exceeds them
	if latency > m.P99JobLatency {
		m.P99JobLatency = latency
	} else if latency > m.P95JobLatency {
		m.P95JobLatency = latency
	}
}
