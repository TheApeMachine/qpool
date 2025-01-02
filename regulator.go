package qpool

/*
Regulator defines an interface for types that regulate the flow and behavior of the pool.
Inspired by biological and mechanical regulators that maintain system homeostasis,
this interface provides a common pattern for implementing various regulation mechanisms.

Each regulator acts as a control system component, monitoring and adjusting the pool's
behavior to maintain optimal performance and stability. Like a thermostat or pressure
regulator in physical systems, these regulators help maintain the system within
desired operational parameters.

Examples of regulators include:
  - CircuitBreaker: Prevents cascading failures by stopping operations when error rates are high
  - RateLimit: Controls the flow rate of jobs to prevent system overload
  - LoadBalancer: Distributes work evenly across available resources
  - BackPressure: Prevents system overload by controlling input rates
  - ResourceGovernor: Manages resource consumption within defined limits
*/
type Regulator interface {
	// Observe allows the regulator to monitor system metrics and state.
	// This is analogous to a sensor in a mechanical regulator, providing
	// the feedback necessary for making control decisions.
	//
	// Parameters:
	//   - metrics: Current system metrics including performance and health indicators
	Observe(metrics *Metrics)

	// Limit determines if the regulated action should be restricted.
	// Returns true if the action should be limited, false if it should proceed.
	// This is the main control point where the regulator decides whether to
	// allow or restrict operations based on observed conditions.
	//
	// Returns:
	//   - bool: true if the action should be limited, false if it should proceed
	Limit() bool

	// Renormalize attempts to return the system to a normal operating state.
	// This is similar to a feedback loop in control systems, where the regulator
	// takes active steps to restore normal operations after a period of restriction.
	// The exact meaning of "normal" depends on the specific regulator implementation.
	Renormalize()
}

/*
NewRegulator creates a new regulator of the specified type.
This factory function allows for flexible creation of different regulator types
while maintaining a consistent interface for the pool to interact with.

Parameters:
  - regulatorType: A concrete implementation of the Regulator interface

Returns:
  - Regulator: The initialized regulator instance

Example:
    circuitBreaker := NewCircuitBreakerRegulator(5, time.Minute, 3)
    regulator := NewRegulator(circuitBreaker)
*/
func NewRegulator(regulatorType Regulator) Regulator {
	return regulatorType
}