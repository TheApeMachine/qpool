package qpool

import (
	"math"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResourceGovernorObserveAndThresholds(t *testing.T) {
	Convey("ResourceGovernor reads CPU hint from MetricReading", t, func() {
		rg := NewResourceGovernorRegulator(0.8, 0.99, time.Second)

		rg.Observe(&MetricReading{ResourceUtilization: 0.5})

		cpu, mem := rg.GetResourceUsage()

		So(cpu, ShouldEqual, 0.5)
		So(mem, ShouldBeGreaterThanOrEqualTo, 0.0)
		So(mem, ShouldBeLessThanOrEqualTo, 1.0)
	})

	Convey("Observe memory tracks runtime heap ratio when interval is zero", t, func() {
		rg := NewResourceGovernorRegulator(0.99, 0.99, 0)

		var ms runtime.MemStats

		runtime.ReadMemStats(&ms)

		total := float64(ms.Sys)

		if total <= 0 {
			total = 1
		}

		expected := float64(ms.Alloc) / total

		rg.Observe(&MetricReading{ResourceUtilization: 0.11})

		_, mem := rg.GetResourceUsage()

		So(math.Abs(mem-expected), ShouldBeLessThan, 0.05)
	})

	Convey("Observe refreshes CPU while throttling repeated MemStats when interval is set", t, func() {
		rg := NewResourceGovernorRegulator(0.9, 0.99, 200*time.Millisecond)

		rg.Observe(&MetricReading{ResourceUtilization: 0.2})

		_, mem1 := rg.GetResourceUsage()

		rg.Observe(&MetricReading{ResourceUtilization: 0.35})

		cpu, mem2 := rg.GetResourceUsage()

		So(mem2, ShouldEqual, mem1)
		So(cpu, ShouldEqual, 0.35)
	})

	Convey("Observe elects one memory sampler inside the check interval", t, func() {
		rg := NewResourceGovernorRegulator(0.9, 0.99, time.Second)
		var calls atomic.Int64

		rg.readMemStats = func(memStats *runtime.MemStats) {
			calls.Add(1)
			memStats.Sys = 100
			memStats.Alloc = 25
		}

		rg.Observe(&MetricReading{ResourceUtilization: 0.2})
		rg.Observe(&MetricReading{ResourceUtilization: 0.3})

		cpu, memory := rg.GetResourceUsage()

		So(calls.Load(), ShouldEqual, 1)
		So(cpu, ShouldEqual, 0.3)
		So(memory, ShouldEqual, 0.25)
	})

	Convey("tryStartMemorySample only permits one sampler per timestamp", t, func() {
		rg := NewResourceGovernorRegulator(0.9, 0.99, time.Second)
		now := int64(time.Second)

		So(rg.tryStartMemorySample(now), ShouldBeTrue)
		So(rg.tryStartMemorySample(now), ShouldBeFalse)
		So(rg.tryStartMemorySample(now+int64(time.Second)), ShouldBeTrue)
	})

	Convey("GetThresholds returns configured ceilings", t, func() {
		rg := NewResourceGovernorRegulator(0.72, 0.91, time.Second)

		cpu, mem := rg.GetThresholds()

		So(cpu, ShouldEqual, 0.72)
		So(mem, ShouldEqual, 0.91)
	})

	Convey("Limit triggers when CPU exceeds threshold", t, func() {
		rg := NewResourceGovernorRegulator(0.5, 0.999, 0)

		rg.Observe(&MetricReading{ResourceUtilization: 0.51})

		So(rg.Limit(), ShouldBeTrue)
	})

	Convey("Limit triggers when memory exceeds threshold", t, func() {
		rg := NewResourceGovernorRegulator(0.999, 0.0000001, 0)

		rg.Observe(nil)

		So(rg.Limit(), ShouldBeTrue)
	})

	Convey("Renormalize does not observe twice inside check interval", t, func() {
		rg := NewResourceGovernorRegulator(0.99, 0.99, 500*time.Millisecond)

		read := &MetricReading{ResourceUtilization: 0.42}

		rg.Observe(read)

		cpu0, mem0 := rg.GetResourceUsage()

		rg.Renormalize()
		cpu1, mem1 := rg.GetResourceUsage()

		rg.Renormalize()
		cpu2, mem2 := rg.GetResourceUsage()

		So(cpu1, ShouldEqual, cpu0)
		So(mem1, ShouldEqual, mem0)
		So(cpu2, ShouldEqual, cpu0)
		So(mem2, ShouldEqual, mem0)
	})

	Convey("NewResourceGovernorRegulator stores negative thresholds as configured", t, func() {
		rg := NewResourceGovernorRegulator(-1, -1, time.Second)

		cpu, mem := rg.GetThresholds()

		So(cpu, ShouldEqual, -1)
		So(mem, ShouldEqual, -1)
	})
}
