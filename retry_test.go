package qpool

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExponentialBackoff_NextDelay(t *testing.T) {
	Convey("Given ExponentialBackoff.NextDelay", t, func() {
		backoff := &ExponentialBackoff{Initial: 100 * time.Millisecond}

		cases := []struct {
			attempt int
			want    time.Duration
		}{
			{1, 100 * time.Millisecond},
			{2, 200 * time.Millisecond},
			{3, 400 * time.Millisecond},
			{4, 800 * time.Millisecond},
			{5, 1600 * time.Millisecond},
		}

		for _, row := range cases {
			attempt := row.attempt
			want := row.want

			Convey(fmt.Sprintf("When attempt is %d", attempt), func() {
				So(backoff.NextDelay(attempt), ShouldEqual, want)
			})
		}
	})
}

func BenchmarkExponentialBackoff_NextDelay(b *testing.B) {
	backoff := &ExponentialBackoff{Initial: time.Millisecond}

	for range b.N {
		_ = backoff.NextDelay(4)
	}
}
