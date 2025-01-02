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
		// Initialization is moved inside each Convey block for isolation

		Convey("It should process a job successfully", func() {
			// Initialize pool and worker for this test case
			pool := &Q{
				ctx:     context.Background(),
				workers: make(chan chan Job, 1),
				space:   newQuantumSpace(),
				metrics: NewMetrics(),
			}

			worker := &Worker{
				pool: pool,
				jobs: make(chan Job, 1),
			}

			// Ensure cleanup after test
			Reset(func() {
				close(worker.jobs)
				pool.space.Close() // Assuming Close method exists
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

			time.Sleep(100 * time.Millisecond) // Allow some time for processing

			result := pool.space.Await(job.ID)
			select {
			case <-pool.ctx.Done():
				t.Fatal(timeoutMsg)
			case value := <-result:
				So(value.Error, ShouldBeNil)
				So(value.Value, ShouldEqual, "result")
			}
		})

		Convey("It should handle job timeout", func() {
			// Initialize pool and worker for this test case
			pool := &Q{
				ctx:     context.Background(),
				workers: make(chan chan Job, 1),
				space:   newQuantumSpace(),
				metrics: NewMetrics(),
			}

			worker := &Worker{
				pool: pool,
				jobs: make(chan Job, 1),
			}

			// Ensure cleanup after test
			Reset(func() {
				close(worker.jobs)
				pool.space.Close() // Assuming Close method exists
			})

			// Create a job that will timeout
			job := Job{
				ID: "job_timeout",
				Fn: func() (any, error) {
					time.Sleep(200 * time.Millisecond)
					return nil, nil
				},
				StartTime:    time.Now(),
				TTL:          100 * time.Millisecond, // Set TTL less than sleep to trigger timeout
				Dependencies: []string{},
			}

			worker.jobs <- job
			ctx, cancel := context.WithTimeout(pool.ctx, 100*time.Millisecond)
			defer cancel()
			worker.pool.ctx = ctx

			go worker.run()

			time.Sleep(150 * time.Millisecond) // Allow some time for processing

			result := pool.space.Await(job.ID)
			select {
			case <-pool.ctx.Done():
				t.Fatal(timeoutMsg)
			case value := <-result:
				So(value.Error, ShouldNotBeNil)
				So(value.Error.Error(), ShouldContainSubstring, "timed out")
			}
		})

		Convey("It should not process a job if dependencies are not met", func() {
			// Initialize pool and worker for this test case
			pool := &Q{
				ctx:     context.Background(),
				workers: make(chan chan Job, 1),
				space:   newQuantumSpace(),
				metrics: NewMetrics(),
			}

			worker := &Worker{
				pool: pool,
				jobs: make(chan Job, 1),
			}

			// Ensure cleanup after test
			Reset(func() {
				close(worker.jobs)
				pool.space.Close() // Assuming Close method exists
			})

			// Add a dependency that is not met
			job := Job{
				ID:           "job_dependency",
				Fn:           func() (any, error) { return "result", nil },
				StartTime:    time.Now(),
				TTL:          10 * time.Second,
				Dependencies: []string{"dep1"},
			}

			worker.jobs <- job
			go worker.run()

			time.Sleep(100 * time.Millisecond) // Allow some time for processing

			result := pool.space.Await(job.ID)
			select {
			case <-pool.ctx.Done():
				t.Fatal(timeoutMsg)
			case value := <-result:
				So(value.Error, ShouldNotBeNil)
				So(value.Error.Error(), ShouldContainSubstring, "dependency dep1 failed")
			}
		})
	})
}
