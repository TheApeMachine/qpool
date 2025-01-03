package qpool

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewLoadBalancer(t *testing.T) {
	Convey("Given parameters for a new load balancer", t, func() {
		workerCount := 5
		workerCapacity := 10

		Convey("When creating a new load balancer", func() {
			balancer := NewLoadBalancer(workerCount, workerCapacity)

			Convey("It should be properly initialized", func() {
				So(balancer, ShouldNotBeNil)
				So(balancer.activeWorkers, ShouldEqual, workerCount)
				So(len(balancer.workerLoads), ShouldEqual, workerCount)
				So(len(balancer.workerLatency), ShouldEqual, workerCount)
				So(len(balancer.workerCapacity), ShouldEqual, workerCount)

				for i := 0; i < workerCount; i++ {
					So(balancer.workerCapacity[i], ShouldEqual, workerCapacity)
					So(balancer.workerLoads[i], ShouldEqual, 0.0)
					So(balancer.workerLatency[i], ShouldEqual, time.Duration(0))
				}
			})
		})
	})
}

func TestLoadBalancerObserve(t *testing.T) {
	Convey("Given a load balancer", t, func() {
		balancer := NewLoadBalancer(3, 5)

		Convey("When observing metrics", func() {
			metrics := &Metrics{
				WorkerCount: 4, // Different from initial count
			}
			balancer.Observe(metrics)

			Convey("It should update its state based on metrics", func() {
				So(balancer.metrics, ShouldEqual, metrics)
				So(balancer.activeWorkers, ShouldEqual, 4)
				So(len(balancer.workerLoads), ShouldBeGreaterThanOrEqualTo, 4)
				So(len(balancer.workerLatency), ShouldBeGreaterThanOrEqualTo, 4)
				So(len(balancer.workerCapacity), ShouldBeGreaterThanOrEqualTo, 4)
			})
		})
	})
}

func TestLoadBalancerLimit(t *testing.T) {
	Convey("Given a load balancer", t, func() {
		balancer := NewLoadBalancer(2, 3)

		Convey("When all workers are below capacity", func() {
			balancer.workerLoads[0] = 2.0
			balancer.workerLoads[1] = 1.0

			Convey("It should not limit", func() {
				So(balancer.Limit(), ShouldBeFalse)
			})
		})

		Convey("When all workers are at capacity", func() {
			balancer.workerLoads[0] = 3.0
			balancer.workerLoads[1] = 3.0

			Convey("It should limit", func() {
				So(balancer.Limit(), ShouldBeTrue)
			})
		})
	})
}

func TestLoadBalancerRenormalize(t *testing.T) {
	Convey("Given a load balancer with incorrect loads", t, func() {
		balancer := NewLoadBalancer(2, 3)
		balancer.workerLoads[0] = 5.0 // Over capacity
		balancer.workerLoads[1] = 4.0 // Over capacity

		Convey("When renormalizing", func() {
			balancer.Renormalize()

			Convey("It should correct the loads", func() {
				So(balancer.workerLoads[0], ShouldEqual, 3.0)
				So(balancer.workerLoads[1], ShouldEqual, 3.0)
			})
		})
	})
}

func TestLoadBalancerSelectWorker(t *testing.T) {
	Convey("Given a load balancer", t, func() {
		balancer := NewLoadBalancer(3, 5)

		Convey("When selecting a worker with available capacity", func() {
			balancer.workerLoads[0] = 4.0
			balancer.workerLoads[1] = 2.0
			balancer.workerLoads[2] = 3.0

			workerID, err := balancer.SelectWorker()

			Convey("It should select the worker with lowest load", func() {
				So(err, ShouldBeNil)
				So(workerID, ShouldEqual, 1)
			})
		})

		Convey("When all workers are at capacity", func() {
			balancer.workerLoads[0] = 5.0
			balancer.workerLoads[1] = 5.0
			balancer.workerLoads[2] = 5.0

			workerID, err := balancer.SelectWorker()

			Convey("It should return an error", func() {
				So(err, ShouldEqual, ErrNoAvailableWorkers)
				So(workerID, ShouldEqual, -1)
			})
		})

		Convey("When workers have equal load but different latencies", func() {
			balancer.workerLoads[0] = 2.0
			balancer.workerLoads[1] = 2.0
			balancer.workerLoads[2] = 2.0
			balancer.workerLatency[0] = 100 * time.Millisecond
			balancer.workerLatency[1] = 50 * time.Millisecond
			balancer.workerLatency[2] = 150 * time.Millisecond

			workerID, err := balancer.SelectWorker()

			Convey("It should select the worker with lower latency", func() {
				So(err, ShouldBeNil)
				So(workerID, ShouldEqual, 1)
			})
		})
	})
}

func TestLoadBalancerRecordJobStart(t *testing.T) {
	Convey("Given a load balancer", t, func() {
		balancer := NewLoadBalancer(2, 5)

		Convey("When recording a job start", func() {
			initialLoad := balancer.workerLoads[0]
			balancer.RecordJobStart(0)

			Convey("It should increment the worker's load", func() {
				So(balancer.workerLoads[0], ShouldEqual, initialLoad+1)
			})
		})

		Convey("When recording a job start for invalid worker", func() {
			invalidWorkerLoad := balancer.workerLoads[0]
			balancer.RecordJobStart(-1)
			balancer.RecordJobStart(2)

			Convey("It should not affect worker loads", func() {
				So(balancer.workerLoads[0], ShouldEqual, invalidWorkerLoad)
			})
		})
	})
}

func TestLoadBalancerRecordJobComplete(t *testing.T) {
	Convey("Given a load balancer with active jobs", t, func() {
		balancer := NewLoadBalancer(2, 5)
		balancer.workerLoads[0] = 2.0

		Convey("When recording a job completion", func() {
			duration := 100 * time.Millisecond
			balancer.RecordJobComplete(0, duration)

			Convey("It should update worker stats", func() {
				So(balancer.workerLoads[0], ShouldEqual, 1.0)
				So(balancer.workerLatency[0], ShouldEqual, duration)
			})
		})

		Convey("When recording multiple job completions", func() {
			balancer.RecordJobComplete(0, 100*time.Millisecond)
			balancer.RecordJobComplete(0, 200*time.Millisecond)

			Convey("It should maintain a moving average of latency", func() {
				So(balancer.workerLatency[0], ShouldEqual, 120*time.Millisecond)
			})
		})

		Convey("When recording more completions than starts", func() {
			balancer.RecordJobComplete(0, 100*time.Millisecond)
			balancer.RecordJobComplete(0, 100*time.Millisecond)
			balancer.RecordJobComplete(0, 100*time.Millisecond)

			Convey("It should not go below zero load", func() {
				So(balancer.workerLoads[0], ShouldEqual, 0.0)
			})
		})
	})
} 