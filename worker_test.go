package qpool

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

const timeoutMsg = "Test timed out waiting for value retrieval"

func TestWorker(t *testing.T) {
	Convey("Given a worker", t, func() {
		Convey("It should process a job successfully", func() {
			pool := &Q{
				ctx:     context.Background(),
				workers: make(chan chan Job, 1),
				space:   NewQSpace(),
				metrics: NewMetrics(),
			}

			worker := &Worker{
				pool: pool,
				jobs: make(chan Job, 1),
			}

			Reset(func() {
				close(worker.jobs)
				pool.space.Close()
			})

			job := Job{
				ID:           "job_success",
				Fn:           func() (any, error) { return "result", nil },
				StartTime:    time.Now(),
				TTL:          10 * time.Second,
				Dependencies: []string{},
			}

			worker.jobs <- job
			go worker.run()

			result := pool.space.Await(job.ID)
			select {
			case <-time.After(2 * time.Second):
				t.Fatal(timeoutMsg)
			case value := <-result:
				So(value.Error, ShouldBeNil)
				So(value.Value, ShouldEqual, "result")
			}
		})

		Convey("It should handle job timeout", func() {
			pool := &Q{
				ctx:     context.Background(),
				workers: make(chan chan Job, 1),
				space:   NewQSpace(),
				metrics: NewMetrics(),
			}

			worker := &Worker{
				pool: pool,
				jobs: make(chan Job, 1),
			}

			Reset(func() {
				close(worker.jobs)
				pool.space.Close()
			})

			job := Job{
				ID: "job_timeout",
				Fn: func() (any, error) {
					time.Sleep(200 * time.Millisecond)
					return nil, nil
				},
				StartTime:    time.Now(),
				TTL:          100 * time.Millisecond,
				Dependencies: []string{},
			}

			ctx, cancel := context.WithTimeout(pool.ctx, 100*time.Millisecond)
			defer cancel()
			worker.pool.ctx = ctx

			
			worker.jobs <- job
			go worker.run()

			result := pool.space.Await(job.ID)
			select {
			case <-time.After(2 * time.Second):
				t.Fatal(timeoutMsg)
			case value := <-result:
				So(value.Error, ShouldNotBeNil)
				So(value.Error.Error(), ShouldContainSubstring, "timed out")
			}
		})

		Convey("It should not process a job if dependencies are not met", func() {
			pool := &Q{
				ctx:     context.Background(),
				workers: make(chan chan Job, 1),
				space:   NewQSpace(),
				metrics: NewMetrics(),
			}

			worker := &Worker{
				pool: pool,
				jobs: make(chan Job, 1),
			}

			Reset(func() {
				close(worker.jobs)
				pool.space.Close()
			})

			job := Job{
				ID:           "job_dependency",
				Fn:           func() (any, error) { return "result", nil },
				StartTime:    time.Now(),
				TTL:          10 * time.Second,
				Dependencies: []string{"dep1"},
			}

			worker.jobs <- job
			go worker.run()

			result := pool.space.Await(job.ID)
			select {
			case <-time.After(2 * time.Second):
				t.Fatal(timeoutMsg)
			case value := <-result:
				So(value.Error, ShouldNotBeNil)
				So(value.Error.Error(), ShouldContainSubstring, "dependency dep1 failed")
			}
		})
	})
}
