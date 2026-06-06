package qpool

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQScheduleFastExecutesJob(test *testing.T) {
	Convey("Given a Q fast path", test, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		pool := NewQ[any](ctx, 1, 2, &Config{Scaler: nil})
		defer pool.Close()
	})
}
