package qpool

import (
	"context"
	"runtime"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDisruptorIdleWaitStrategy(test *testing.T) {
	Convey("Given DisruptorIdleWaitStrategy", test, func() {
		strategy := &DisruptorIdleWaitStrategy{}

		Convey("It should not panic across Gate Idle and Reserve", func() {
			strategy.Gate(1)
			strategy.Idle(1)
			strategy.Idle(512)
			strategy.Idle(4096)
			strategy.Reserve(1)
		})
	})
}

func TestQIdleDisruptorListenersClose(test *testing.T) {
	Convey("Given a Q pool whose disruptor listeners are idle", test, func() {
		ctx, cancel := context.WithCancel(context.Background())
		pool := NewQ[any](ctx, 1, max(4, runtime.NumCPU()), nil)

		time.Sleep(20 * time.Millisecond)

		Convey("It should close without hanging", func() {
			cancel()
			pool.Close()
		})
	})
}
