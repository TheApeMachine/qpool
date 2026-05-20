package qpool

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCircuitBreakerCacheGetOrCreate(test *testing.T) {
	Convey("Given a bounded circuit breaker cache", test, func() {
		cache := newCircuitBreakerCache(2)
		config := &CircuitBreakerConfig{
			MaxFailures:  1,
			ResetTimeout: time.Minute,
			HalfOpenMax:  1,
		}

		Convey("It should return the existing breaker and evict least-recently used entries", func() {
			first := cache.getOrCreate("first", config)
			second := cache.getOrCreate("second", config)

			So(cache.getOrCreate("first", config), ShouldEqual, first)

			third := cache.getOrCreate("third", config)

			So(third, ShouldNotBeNil)
			So(cache.getOrCreate("first", config), ShouldEqual, first)
			So(cache.getOrCreate("second", config) == second, ShouldBeFalse)
		})
	})
}

func TestQBreakerForJob(test *testing.T) {
	Convey("Given an admitted circuit breaker job", test, func() {
		ctx := test.Context()
		pool := NewQ(ctx, 1, 1, &Config{
			CircuitBreakerLimit: 1,
			Scaler:              nil,
		})

		defer pool.Close()

		config := &CircuitBreakerConfig{
			MaxFailures:  1,
			ResetTimeout: time.Minute,
			HalfOpenMax:  1,
		}

		job := &Job{
			CircuitID:     "first",
			CircuitConfig: config,
		}

		breaker := pool.breakerFor(job)
		job.circuitBreaker = breaker

		_ = pool.breakerFor(&Job{
			CircuitID:     "second",
			CircuitConfig: config,
		})

		Convey("It should keep the admitted breaker even after cache eviction", func() {
			So(pool.breakerForJob(job), ShouldEqual, breaker)
		})
	})
}

func BenchmarkCircuitBreakerCacheGetOrCreate(benchmark *testing.B) {
	cache := newCircuitBreakerCache(64)
	config := &CircuitBreakerConfig{
		MaxFailures:  2,
		ResetTimeout: time.Minute,
		HalfOpenMax:  2,
	}

	identifiers := make([]string, 128)

	for index := range identifiers {
		identifiers[index] = fmt.Sprintf("breaker-%d", index)
	}

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	index := 0

	for benchmark.Loop() {
		_ = cache.getOrCreate(identifiers[index%len(identifiers)], config)
		index++
	}
}
