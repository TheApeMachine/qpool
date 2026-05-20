package qpool

import (
	"math"
	"runtime"
	"sync/atomic"
	"time"
)

/*
ResourceGovernorRegulator implements Regulator using atomic gauges updated from MetricReading and runtime memory stats.
*/
type ResourceGovernorRegulator struct {
	maxCPUPercent    float64
	maxMemoryPercent float64
	checkIntervalNs  int64

	currentCPU              atomic.Uint64
	currentMemory           atomic.Uint64
	lastReading             atomic.Pointer[MetricReading]
	lastRenormalizeUnixNano atomic.Int64
	lastMemSampleUnixNano   atomic.Int64
	readMemStats            func(*runtime.MemStats)
}

/*
NewResourceGovernorRegulator constructs a resource governor.
*/
func NewResourceGovernorRegulator(maxCPUPercent, maxMemoryPercent float64, checkInterval time.Duration) *ResourceGovernorRegulator {
	return &ResourceGovernorRegulator{
		maxCPUPercent:    maxCPUPercent,
		maxMemoryPercent: maxMemoryPercent,
		checkIntervalNs:  checkInterval.Nanoseconds(),
		readMemStats:     runtime.ReadMemStats,
	}
}

/*
Observe refreshes CPU from reading when present and updates the memory ratio from runtime.MemStats.

When checkInterval is positive, ReadMemStats runs at most once per interval; intervening calls reuse the cached memory fraction while still applying CPU hints from the latest reading.
*/
func (rg *ResourceGovernorRegulator) Observe(reading *MetricReading) {
	if reading != nil {
		rg.lastReading.Store(reading)

		if reading.ResourceUtilization > 0 {
			rg.currentCPU.Store(math.Float64bits(reading.ResourceUtilization))
		}
	}

	now := time.Now().UnixNano()

	if !rg.tryStartMemorySample(now) {
		return
	}

	var memStats runtime.MemStats

	rg.readRuntimeMemStats(&memStats)

	totalMemory := float64(memStats.Sys)
	if totalMemory <= 0 {
		totalMemory = 1
	}

	usedMemory := float64(memStats.Alloc)
	rg.currentMemory.Store(math.Float64bits(usedMemory / totalMemory))
}

func (rg *ResourceGovernorRegulator) tryStartMemorySample(now int64) bool {
	if rg.checkIntervalNs <= 0 {
		return true
	}

	lastMem := rg.lastMemSampleUnixNano.Load()

	if lastMem != 0 && now-lastMem < rg.checkIntervalNs {
		return false
	}

	return rg.lastMemSampleUnixNano.CompareAndSwap(lastMem, now)
}

func (rg *ResourceGovernorRegulator) readRuntimeMemStats(memStats *runtime.MemStats) {
	if rg.readMemStats == nil {
		runtime.ReadMemStats(memStats)

		return
	}

	rg.readMemStats(memStats)
}

/*
Limit returns true when CPU or memory exceeds configured thresholds.
*/
func (rg *ResourceGovernorRegulator) Limit() bool {
	cpu := math.Float64frombits(rg.currentCPU.Load())
	mem := math.Float64frombits(rg.currentMemory.Load())

	return cpu >= rg.maxCPUPercent || mem >= rg.maxMemoryPercent
}

/*
Renormalize re-applies Observe using the last reading at most once per check interval.

When checkInterval is zero, Renormalize always observes immediately when a reading exists.
*/
func (rg *ResourceGovernorRegulator) Renormalize() {
	read := rg.lastReading.Load()
	if read == nil {
		return
	}

	if rg.checkIntervalNs <= 0 {
		rg.Observe(read)

		return
	}

	now := time.Now().UnixNano()
	prev := rg.lastRenormalizeUnixNano.Load()

	if prev != 0 && now-prev < rg.checkIntervalNs {
		return
	}

	if !rg.lastRenormalizeUnixNano.CompareAndSwap(prev, now) {
		return
	}

	rg.Observe(read)
}

/*
GetResourceUsage returns the latest CPU and memory ratios.
*/
func (rg *ResourceGovernorRegulator) GetResourceUsage() (cpu, memory float64) {
	return math.Float64frombits(rg.currentCPU.Load()),
		math.Float64frombits(rg.currentMemory.Load())
}

/*
GetThresholds returns configured thresholds.
*/
func (rg *ResourceGovernorRegulator) GetThresholds() (cpu, memory float64) {
	return rg.maxCPUPercent, rg.maxMemoryPercent
}
