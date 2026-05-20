package qpool

import (
	"time"
)

/*
Config controls timeouts, admission regulators, queue buffering, and optional periodic scaling.
*/
type Config struct {
	SchedulingTimeout  time.Duration
	Regulators         []Regulator
	JobChannelCapacity int
	// CircuitBreakerLimit bounds the per-pool circuit breaker LRU.
	CircuitBreakerLimit int
	Scaler              *ScalerConfig

	/*
		TelemetryPublish forwards pool-originated events into the app’s telemetry
		sink (for example telemetry.Publish) without qpool importing telemetry.
	*/
	TelemetryPublish func(Event)
}

/*
NewConfig returns defaults including an enabled periodic scaler.

Set Scaler to nil to disable the built-in scaling goroutine (for example when using AdaptiveScalerRegulator alone).
*/
func NewConfig() *Config {
	return &Config{
		SchedulingTimeout:   10 * time.Second,
		CircuitBreakerLimit: defaultCircuitBreakerLimit,
		Scaler: &ScalerConfig{
			TargetLoad:         2.0,
			ScaleUpThreshold:   4.0,
			ScaleDownThreshold: 1.0,
			Cooldown:           500 * time.Millisecond,
			Interval:           time.Second,
		},
	}
}
