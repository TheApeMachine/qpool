package qpool

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewRateLimiter(t *testing.T) {
	Convey("Given a new rate limiter", t, func() {
		limiter := NewRateLimiter(100, time.Second)
		
		Convey("It should be properly initialized", func() {
			So(limiter, ShouldNotBeNil)
			So(limiter.tokens, ShouldEqual, 100)
			So(limiter.maxTokens, ShouldEqual, 100)
			So(limiter.refillRate, ShouldEqual, time.Second)
		})
	})
}

func TestRateLimiterObserve(t *testing.T) {
	Convey("Given a rate limiter", t, func() {
		limiter := NewRateLimiter(100, time.Second)
		metrics := &Metrics{}
		
		Convey("When observing metrics", func() {
			limiter.Observe(metrics)
			So(limiter.metrics, ShouldEqual, metrics)
		})
	})
}

func TestRateLimiterLimit(t *testing.T) {
	Convey("Given a rate limiter with 2 tokens", t, func() {
		limiter := NewRateLimiter(2, time.Second)
		
		Convey("When consuming tokens", func() {
			// First two requests should succeed
			So(limiter.Limit(), ShouldBeFalse)
			So(limiter.Limit(), ShouldBeFalse)
			
			// Third request should be limited
			So(limiter.Limit(), ShouldBeTrue)
		})
	})
}

func TestRateLimiterBurst(t *testing.T) {
	Convey("Given a rate limiter with burst capacity", t, func() {
		limiter := NewRateLimiter(3, 100*time.Millisecond)
		
		Convey("It should handle burst and refill", func() {
			// Use all tokens in burst
			So(limiter.Limit(), ShouldBeFalse)
			So(limiter.Limit(), ShouldBeFalse)
			So(limiter.Limit(), ShouldBeFalse)
			So(limiter.Limit(), ShouldBeTrue)
			
			// Wait for refill
			time.Sleep(150 * time.Millisecond)
			
			// Should have tokens again
			So(limiter.Limit(), ShouldBeFalse)
		})
	})
}

func TestRateLimiterRefill(t *testing.T) {
	Convey("Given a rate limiter", t, func() {
		limiter := NewRateLimiter(5, 100*time.Millisecond)
		
		Convey("It should refill tokens over time", func() {
			// Use some tokens
			So(limiter.Limit(), ShouldBeFalse)
			So(limiter.Limit(), ShouldBeFalse)
			So(limiter.tokens, ShouldEqual, 3)
			
			// Wait for refill period
			time.Sleep(150 * time.Millisecond)
			
			// Force refill check
			limiter.Renormalize()
			
			// Should be refilled
			So(limiter.tokens, ShouldEqual, 5)
		})
	})
}

func TestRateLimiterRenormalize(t *testing.T) {
	Convey("Given a rate limiter", t, func() {
		limiter := NewRateLimiter(2, 100*time.Millisecond)
		
		Convey("When renormalizing", func() {
			// Use all tokens
			So(limiter.Limit(), ShouldBeFalse)
			So(limiter.Limit(), ShouldBeFalse)
			So(limiter.tokens, ShouldEqual, 0)
			
			// Wait and renormalize
			time.Sleep(150 * time.Millisecond)
			limiter.Renormalize()
			
			// Should have tokens again
			So(limiter.tokens, ShouldEqual, 2)
		})
	})
}
