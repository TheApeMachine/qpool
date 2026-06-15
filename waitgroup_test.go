package qpool

import (
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWaitGroupWait(test *testing.T) {
	Convey("Given a WaitGroup with pending work", test, func() {
		waitGroup := &WaitGroup{}
		waitGroup.Add(1)

		done := make(chan struct{})

		go func() {
			waitGroup.Wait()
			close(done)
		}()

		time.Sleep(25 * time.Millisecond)

		select {
		case <-done:
			test.Fatal("Wait returned before Done")
		default:
		}

		waitGroup.Done()

		Convey("It should unblock after Done reaches zero", func() {
			select {
			case <-done:
			case <-time.After(time.Second):
				test.Fatal("timed out waiting for WaitGroup")
			}
		})
	})
}

func TestWaitGroupWaitMultipleDone(test *testing.T) {
	Convey("Given a WaitGroup tracking multiple goroutines", test, func() {
		waitGroup := &WaitGroup{}
		waitGroup.Add(3)

		var completed atomic.Int64

		for range 3 {
			go func() {
				defer waitGroup.Done()

				time.Sleep(10 * time.Millisecond)
			}()
		}

		waitGroup.Wait()

		Convey("It should return only after every Done", func() {
			So(waitGroup.count.Load(), ShouldEqual, 0)
			completed.Store(1)
			So(completed.Load(), ShouldEqual, 1)
		})
	})
}

func BenchmarkWaitGroupWaitDone(benchmark *testing.B) {
	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for benchmark.Loop() {
		waitGroup := &WaitGroup{}
		waitGroup.Add(1)

		go func() {
			waitGroup.Done()
		}()

		waitGroup.Wait()
	}
}
