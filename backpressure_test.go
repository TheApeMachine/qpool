package qpool

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewBackPressureRegulator(t *testing.T) {
	Convey("Given parameters for a new back pressure regulator", t, func() {
		maxQueueSize := 1000
		targetProcessTime := time.Second
		pressureWindow := time.Minute

		Convey("When creating a new back pressure regulator", func() {
			regulator := NewBackPressureRegulator(maxQueueSize, targetProcessTime, pressureWindow)

			Convey("It should be properly initialized", func() {
				So(regulator, ShouldNotBeNil)
				So(regulator.maxQueueSize, ShouldEqual, maxQueueSize)
				So(regulator.targetProcessTime, ShouldEqual, targetProcessTime)
				So(regulator.pressureWindow, ShouldEqual, pressureWindow)
				So(regulator.currentPressure, ShouldEqual, 0.0)
				So(regulator.metrics, ShouldBeNil)
			})
		})
	})
}

func TestBackPressureObserve(t *testing.T) {
	Convey("Given a back pressure regulator", t, func() {
		regulator := NewBackPressureRegulator(100, time.Second, time.Minute)

		Convey("When observing metrics with high queue size", func() {
			metrics := &Metrics{
				JobQueueSize:      80,  // 80% of max
				AverageJobLatency: time.Second * 2, // 200% of target
			}
			regulator.Observe(metrics)

			Convey("It should update pressure accordingly", func() {
				So(regulator.metrics, ShouldEqual, metrics)
				// Pressure = (0.8 * 0.6) + (2.0 * 0.4) = 0.48 + 0.8 = 1.28, capped at 1.0
				So(regulator.currentPressure, ShouldEqual, 1.0)
			})
		})

		Convey("When observing metrics with low utilization", func() {
			metrics := &Metrics{
				JobQueueSize:      20,  // 20% of max
				AverageJobLatency: time.Second / 2, // 50% of target
			}
			regulator.Observe(metrics)

			Convey("It should show low pressure", func() {
				// Pressure = (0.2 * 0.6) + (0.5 * 0.4) = 0.12 + 0.2 = 0.32
				So(regulator.currentPressure, ShouldBeLessThan, 0.4)
			})
		})
	})
}

func TestBackPressureLimit(t *testing.T) {
	Convey("Given a back pressure regulator", t, func() {
		regulator := NewBackPressureRegulator(100, time.Second, time.Minute)

		Convey("When pressure is below threshold", func() {
			regulator.currentPressure = 0.5 // 50% pressure

			Convey("It should not limit", func() {
				So(regulator.Limit(), ShouldBeFalse)
			})
		})

		Convey("When pressure is above threshold", func() {
			regulator.currentPressure = 0.9 // 90% pressure

			Convey("It should limit", func() {
				So(regulator.Limit(), ShouldBeTrue)
			})
		})

		Convey("When at threshold", func() {
			regulator.currentPressure = 0.8 // 80% pressure (threshold)

			Convey("It should limit", func() {
				So(regulator.Limit(), ShouldBeTrue)
			})
		})
	})
}

func TestBackPressureRenormalize(t *testing.T) {
	Convey("Given a back pressure regulator under high pressure", t, func() {
		regulator := NewBackPressureRegulator(100, time.Second, time.Minute)
		regulator.currentPressure = 0.9

		Convey("When conditions are good for renormalization", func() {
			metrics := &Metrics{
				JobQueueSize:      30,  // Below 50% of max
				AverageJobLatency: time.Second / 2, // Below target
			}
			regulator.metrics = metrics
			regulator.Renormalize()

			Convey("It should reduce pressure", func() {
				So(regulator.currentPressure, ShouldEqual, 0.8) // Reduced by 0.1
			})
		})

		Convey("When conditions are not good for renormalization", func() {
			metrics := &Metrics{
				JobQueueSize:      80,  // Above 50% of max
				AverageJobLatency: time.Second * 2, // Above target
			}
			regulator.metrics = metrics
			regulator.Renormalize()

			Convey("It should maintain pressure", func() {
				So(regulator.currentPressure, ShouldEqual, 0.9) // Unchanged
			})
		})
	})
}

func TestBackPressureGetPressure(t *testing.T) {
	Convey("Given a back pressure regulator", t, func() {
		regulator := NewBackPressureRegulator(100, time.Second, time.Minute)
		expectedPressure := 0.75
		regulator.currentPressure = expectedPressure

		Convey("When getting current pressure", func() {
			pressure := regulator.GetPressure()

			Convey("It should return the correct value", func() {
				So(pressure, ShouldEqual, expectedPressure)
			})
		})
	})
}

func TestBackPressureUpdatePressure(t *testing.T) {
	Convey("Given a back pressure regulator", t, func() {
		regulator := NewBackPressureRegulator(100, time.Second, time.Minute)

		Convey("When updating pressure with nil metrics", func() {
			regulator.currentPressure = 0.5
			regulator.updatePressure()

			Convey("It should maintain current pressure", func() {
				So(regulator.currentPressure, ShouldEqual, 0.5)
			})
		})

		Convey("When updating pressure with zero latency", func() {
			metrics := &Metrics{
				JobQueueSize:      50, // 50% of max
				AverageJobLatency: 0,
			}
			regulator.metrics = metrics
			regulator.updatePressure()

			Convey("It should calculate pressure from queue size only", func() {
				// Pressure = (0.5 * 0.6) + (0.0 * 0.4) = 0.3
				So(regulator.currentPressure, ShouldEqual, 0.3)
			})
		})

		Convey("When updating pressure with all metrics", func() {
			metrics := &Metrics{
				JobQueueSize:      75,  // 75% of max
				AverageJobLatency: 3 * time.Second / 2, // 150% of target
			}
			regulator.metrics = metrics
			regulator.updatePressure()

			Convey("It should calculate combined pressure correctly", func() {
				// Pressure = (0.75 * 0.6) + (1.5 * 0.4) = 0.45 + 0.6 = 1.05, capped at 1.0
				So(regulator.currentPressure, ShouldEqual, 1.0)
			})
		})
	})
} 