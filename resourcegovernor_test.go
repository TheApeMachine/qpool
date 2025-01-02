package qpool

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewResourceGovernorRegulator(t *testing.T) {
	Convey("Given parameters for a new resource governor", t, func() {
		maxCPU := 0.8
		maxMemory := 0.9
		checkInterval := time.Second

		Convey("When creating a new resource governor", func() {
			governor := NewResourceGovernorRegulator(maxCPU, maxMemory, checkInterval)

			Convey("It should be properly initialized", func() {
				So(governor, ShouldNotBeNil)
				So(governor.maxCPUPercent, ShouldEqual, maxCPU)
				So(governor.maxMemoryPercent, ShouldEqual, maxMemory)
				So(governor.checkInterval, ShouldEqual, checkInterval)
				So(governor.currentCPU, ShouldEqual, 0.0)
				So(governor.currentMemory, ShouldEqual, 0.0)
				So(governor.metrics, ShouldBeNil)
			})
		})
	})
}

func TestResourceGovernorObserve(t *testing.T) {
	Convey("Given a resource governor", t, func() {
		governor := NewResourceGovernorRegulator(0.8, 0.9, time.Second)

		Convey("When observing metrics with high resource usage", func() {
			metrics := &Metrics{
				ResourceUtilization: 0.85, // 85% CPU
			}
			governor.Observe(metrics)

			Convey("It should update resource usage", func() {
				So(governor.metrics, ShouldEqual, metrics)
				So(governor.currentCPU, ShouldEqual, 0.85)
				// Memory is updated via runtime.ReadMemStats, so we don't test the exact value
				So(governor.currentMemory, ShouldBeLessThan, 1.0)
			})
		})

		Convey("When observing metrics with low resource usage", func() {
			metrics := &Metrics{
				ResourceUtilization: 0.3, // 30% CPU
			}
			governor.Observe(metrics)

			Convey("It should update resource usage", func() {
				So(governor.currentCPU, ShouldEqual, 0.3)
				So(governor.currentMemory, ShouldBeLessThan, 1.0)
			})
		})
	})
}

func TestResourceGovernorLimit(t *testing.T) {
	Convey("Given a resource governor", t, func() {
		governor := NewResourceGovernorRegulator(0.8, 0.9, time.Second)

		Convey("When resources are below thresholds", func() {
			governor.currentCPU = 0.7    // 70% CPU
			governor.currentMemory = 0.8  // 80% Memory

			Convey("It should not limit", func() {
				So(governor.Limit(), ShouldBeFalse)
			})
		})

		Convey("When CPU is above threshold", func() {
			governor.currentCPU = 0.85    // 85% CPU
			governor.currentMemory = 0.8   // 80% Memory

			Convey("It should limit", func() {
				So(governor.Limit(), ShouldBeTrue)
			})
		})

		Convey("When memory is above threshold", func() {
			governor.currentCPU = 0.7     // 70% CPU
			governor.currentMemory = 0.95  // 95% Memory

			Convey("It should limit", func() {
				So(governor.Limit(), ShouldBeTrue)
			})
		})

		Convey("When both resources are above thresholds", func() {
			governor.currentCPU = 0.85    // 85% CPU
			governor.currentMemory = 0.95  // 95% Memory

			Convey("It should limit", func() {
				So(governor.Limit(), ShouldBeTrue)
			})
		})
	})
}

func TestResourceGovernorRenormalize(t *testing.T) {
	Convey("Given a resource governor", t, func() {
		governor := NewResourceGovernorRegulator(0.8, 0.9, time.Second)

		Convey("When renormalizing with metrics", func() {
			metrics := &Metrics{
				ResourceUtilization: 0.5, // 50% CPU
			}
			governor.metrics = metrics
			governor.currentCPU = 0.85    // Old high value
			governor.Renormalize()

			Convey("It should update resource measurements", func() {
				So(governor.currentCPU, ShouldEqual, 0.5)
			})
		})

		Convey("When renormalizing without metrics", func() {
			governor.metrics = nil
			governor.currentCPU = 0.85
			governor.Renormalize()

			Convey("It should maintain current values", func() {
				So(governor.currentCPU, ShouldEqual, 0.85)
			})
		})
	})
}

func TestResourceGovernorGetResourceUsage(t *testing.T) {
	Convey("Given a resource governor with known usage", t, func() {
		governor := NewResourceGovernorRegulator(0.8, 0.9, time.Second)
		governor.currentCPU = 0.75
		governor.currentMemory = 0.65

		Convey("When getting resource usage", func() {
			cpu, memory := governor.GetResourceUsage()

			Convey("It should return correct values", func() {
				So(cpu, ShouldEqual, 0.75)
				So(memory, ShouldEqual, 0.65)
			})
		})
	})
}

func TestResourceGovernorGetThresholds(t *testing.T) {
	Convey("Given a resource governor with known thresholds", t, func() {
		maxCPU := 0.8
		maxMemory := 0.9
		governor := NewResourceGovernorRegulator(maxCPU, maxMemory, time.Second)

		Convey("When getting thresholds", func() {
			cpu, memory := governor.GetThresholds()

			Convey("It should return correct values", func() {
				So(cpu, ShouldEqual, maxCPU)
				So(memory, ShouldEqual, maxMemory)
			})
		})
	})
}

func TestResourceGovernorUpdateResourceUsage(t *testing.T) {
	Convey("Given a resource governor", t, func() {
		governor := NewResourceGovernorRegulator(0.8, 0.9, time.Second)

		Convey("When updating resource usage with nil metrics", func() {
			governor.currentCPU = 0.5
			governor.metrics = nil
			governor.updateResourceUsage()

			Convey("It should maintain current values", func() {
				So(governor.currentCPU, ShouldEqual, 0.5)
			})
		})

		Convey("When updating resource usage with metrics", func() {
			metrics := &Metrics{
				ResourceUtilization: 0.6, // 60% CPU
			}
			governor.metrics = metrics
			governor.updateResourceUsage()

			Convey("It should update CPU usage", func() {
				So(governor.currentCPU, ShouldEqual, 0.6)
				So(governor.currentMemory, ShouldBeLessThan, 1.0)
			})
		})

		Convey("When updating with zero resource utilization", func() {
			metrics := &Metrics{
				ResourceUtilization: 0.0,
			}
			governor.currentCPU = 0.5
			governor.metrics = metrics
			governor.updateResourceUsage()

			Convey("It should maintain current CPU value", func() {
				So(governor.currentCPU, ShouldEqual, 0.5)
			})
		})
	})
} 