package qpool

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDependenciesConfiguredByOption(t *testing.T) {
	Convey("Given DependenciesConfiguredByOption", t, func() {
		cases := []struct {
			name string
			opt  JobOption
			want bool
		}{
			{name: "nil option", opt: nil, want: false},
			{name: "WithRetry does not set dependencies", opt: WithRetry(2, &ExponentialBackoff{Initial: time.Millisecond}), want: false},
			{name: "WithCircuitBreaker does not set dependencies", opt: WithCircuitBreaker("cb", 2, time.Minute), want: false},
			{name: "WithDependencyRetry does not set dependencies", opt: WithDependencyRetry(2, &ExponentialBackoff{Initial: time.Millisecond}), want: false},
			{name: "WithDependencies nil slice", opt: WithDependencies(nil), want: false},
			{name: "WithDependencies empty slice", opt: WithDependencies([]string{}), want: false},
			{name: "WithDependencies single id", opt: WithDependencies([]string{"a"}), want: true},
			{name: "WithDependencies multiple ids", opt: WithDependencies([]string{"x", "y"}), want: true},
		}

		for _, row := range cases {
			label := row.name

			Convey(fmt.Sprintf("When %s", label), func() {
				So(DependenciesConfiguredByOption(row.opt), ShouldEqual, row.want)
			})
		}
	})
}

func TestWithDependencies(t *testing.T) {
	Convey("Given WithDependencies", t, func() {
		cases := []struct {
			name string
			in   []string
			want []string
		}{
			{name: "nil slice clears dependencies", in: nil, want: nil},
			{name: "empty slice clears dependencies", in: []string{}, want: nil},
			{name: "copies non-empty slice", in: []string{"p", "q"}, want: []string{"p", "q"}},
		}

		for _, row := range cases {
			label := row.name

			Convey(fmt.Sprintf("When %s", label), func() {
				var job Job

				opt := WithDependencies(row.in)
				opt(&job)

				So(job.Dependencies, ShouldResemble, row.want)

				if len(row.in) > 0 {
					job.Dependencies[0] = "mutated"

					So(row.in[0], ShouldNotEqual, "mutated")
				}
			})
		}
	})
}

func TestWithRetry(t *testing.T) {
	Convey("Given WithRetry", t, func() {
		strategy := &ExponentialBackoff{Initial: 7 * time.Millisecond}

		cases := []struct {
			name     string
			attempts int
			strategy RetryStrategy
			wantMax  int
		}{
			{name: "three attempts", attempts: 3, strategy: strategy, wantMax: 3},
			{name: "zero attempts still attaches policy", attempts: 0, strategy: strategy, wantMax: 0},
		}

		for _, row := range cases {
			label := row.name

			Convey(fmt.Sprintf("When %s", label), func() {
				var job Job

				WithRetry(row.attempts, row.strategy)(&job)

				So(job.RetryPolicy, ShouldNotBeNil)
				So(job.RetryPolicy.MaxAttempts, ShouldEqual, row.wantMax)
				So(job.RetryPolicy.Strategy, ShouldEqual, row.strategy)
			})
		}
	})
}

func TestWithCircuitBreaker(t *testing.T) {
	Convey("Given WithCircuitBreaker", t, func() {
		var job Job

		reset := 88 * time.Second

		WithCircuitBreaker("lane-a", 5, reset)(&job)

		So(job.CircuitID, ShouldEqual, "lane-a")
		So(job.CircuitConfig, ShouldNotBeNil)
		So(job.CircuitConfig.MaxFailures, ShouldEqual, 5)
		So(job.CircuitConfig.ResetTimeout, ShouldEqual, reset)
		So(job.CircuitConfig.HalfOpenMax, ShouldEqual, 2)
	})
}

func TestWithDependencyRetry(t *testing.T) {
	Convey("Given WithDependencyRetry", t, func() {
		strategy := &ExponentialBackoff{Initial: 11 * time.Millisecond}

		cases := []struct {
			name     string
			attempts int
			wantMax  int
		}{
			{name: "four attempts", attempts: 4, wantMax: 4},
			{name: "single attempt", attempts: 1, wantMax: 1},
		}

		for _, row := range cases {
			label := row.name

			Convey(fmt.Sprintf("When %s", label), func() {
				var job Job

				WithDependencyRetry(row.attempts, strategy)(&job)

				So(job.DependencyRetryPolicy, ShouldNotBeNil)
				So(job.DependencyRetryPolicy.MaxAttempts, ShouldEqual, row.wantMax)
				So(job.DependencyRetryPolicy.Strategy, ShouldEqual, strategy)
			})
		}
	})
}
