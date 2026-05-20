package qpool

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLoadBalancer_Limit(t *testing.T) {
	Convey("Given LoadBalancer.Limit", t, func() {
		cases := []struct {
			name      string
			capacity  int
			reading   *MetricReading
			wantLimit bool
		}{
			{
				name:      "nil reading does not limit",
				capacity:  3,
				reading:   nil,
				wantLimit: false,
			},
			{
				name:      "below capacity per worker does not limit",
				capacity:  3,
				reading:   &MetricReading{WorkerCount: 2, JobQueueSize: 5},
				wantLimit: false,
			},
			{
				name:      "at capacity per worker limits",
				capacity:  3,
				reading:   &MetricReading{WorkerCount: 2, JobQueueSize: 6},
				wantLimit: true,
			},
			{
				name:      "above capacity per worker limits",
				capacity:  3,
				reading:   &MetricReading{WorkerCount: 2, JobQueueSize: 10},
				wantLimit: true,
			},
			{
				name:      "zero worker count uses one worker for load math",
				capacity:  3,
				reading:   &MetricReading{WorkerCount: 0, JobQueueSize: 4},
				wantLimit: true,
			},
		}

		for _, row := range cases {
			label := row.name

			Convey(fmt.Sprintf("When %s", label), func() {
				loadBalancer := NewLoadBalancer(99, row.capacity)

				if row.reading != nil {
					loadBalancer.Observe(row.reading)
				}

				So(loadBalancer.Limit(), ShouldEqual, row.wantLimit)
			})
		}
	})
}

func TestLoadBalancerObserveAndLimit(t *testing.T) {
	Convey("LoadBalancer limits when queue depth per worker exceeds capacity", t, func() {
		lb := NewLoadBalancer(2, 3)

		So(lb.Limit(), ShouldBeFalse)

		lb.Observe(&MetricReading{WorkerCount: 2, JobQueueSize: 8})

		So(lb.Limit(), ShouldBeTrue)
	})
}

func TestLoadBalancer_SelectWorker(t *testing.T) {
	Convey("Given LoadBalancer.SelectWorker", t, func() {
		cases := []struct {
			name        string
			reading     *MetricReading
			wantWorker  int
			wantFailure bool
		}{
			{
				name:        "nil reading returns ErrNoAvailableWorkers",
				reading:     nil,
				wantWorker:  -1,
				wantFailure: true,
			},
			{
				name:        "zero WorkerCount returns ErrNoAvailableWorkers",
				reading:     &MetricReading{WorkerCount: 0, TotalJobs: 5},
				wantWorker:  -1,
				wantFailure: true,
			},
			{
				name:        "negative WorkerCount returns ErrNoAvailableWorkers",
				reading:     &MetricReading{WorkerCount: -2, TotalJobs: 3},
				wantWorker:  -1,
				wantFailure: true,
			},
			{
				name:        "positive WorkerCount returns TotalJobs modulo WorkerCount",
				reading:     &MetricReading{WorkerCount: 3, JobQueueSize: 2, TotalJobs: 10},
				wantWorker:  1,
				wantFailure: false,
			},
			{
				name:        "modulo zero total jobs",
				reading:     &MetricReading{WorkerCount: 4, TotalJobs: 12},
				wantWorker:  0,
				wantFailure: false,
			},
		}

		for _, row := range cases {
			label := row.name

			Convey(fmt.Sprintf("When %s", label), func() {
				loadBalancer := NewLoadBalancer(9, 5)

				if row.reading != nil {
					loadBalancer.Observe(row.reading)
				}

				workerID, err := loadBalancer.SelectWorker()

				So(workerID, ShouldEqual, row.wantWorker)

				if row.wantFailure {
					So(err, ShouldEqual, ErrNoAvailableWorkers)
				}

				if !row.wantFailure {
					So(err, ShouldBeNil)
				}
			})
		}
	})
}
