# Regulator Interface

The Regulator interface is the cornerstone of QPool's regulation system, providing a unified way to implement various control mechanisms that help maintain system stability and performance.

## Overview

Like biological and mechanical control systems, QPool's regulators observe system metrics, make decisions based on those observations, and take corrective actions when necessary. The Regulator interface defines three key methods that enable this feedback loop:

```go
type Regulator interface {
    Observe(*Metrics)
    Limit() bool
    Renormalize()
}
```

## Core Concepts

### Observation (Observe method)

The `Observe` method is where regulators receive and process system metrics. This is analogous to how a thermostat reads room temperature or how your body's receptors detect changes in blood sugar levels. Regulators can:

- Process current system metrics
- Update internal state
- Calculate derived metrics
- Store historical data for trend analysis

### Limitation (Limit method)

The `Limit` method determines whether the system should restrict operations based on current conditions. This is similar to how a circuit breaker trips when detecting a power surge. The method:

- Returns `true` if operations should be limited
- Returns `false` if operations can proceed normally
- Makes decisions based on internal state and thresholds

### Renormalization (Renormalize method)

The `Renormalize` method provides a way to adjust the regulator's internal state when conditions improve. Like how your body returns to homeostasis after stress, this method:

- Reduces pressure or limitations gradually
- Prevents oscillation between states
- Implements recovery logic

## Built-in Regulators

QPool provides several built-in regulators, each specializing in different aspects of system control:

- [RateLimiter](ratelimiter.md): Controls operation rates
- [CircuitBreaker](circuitbreaker.md): Prevents cascading failures
- [LoadBalancer](loadbalancer.md): Distributes work efficiently
- [BackPressureRegulator](backpressure.md): Manages system pressure
- [ResourceGovernorRegulator](resourcegovernor.md): Controls resource usage
- [AdaptiveScalerRegulator](adaptivescaler.md): Optimizes worker pool size

## Creating Custom Regulators

You can create custom regulators by implementing the Regulator interface. Here's a simple example:

```go
type CustomRegulator struct {
    threshold float64
    current   float64
}

func (r *CustomRegulator) Observe(m *Metrics) {
    r.current = m.ResourceUtilization
}

func (r *CustomRegulator) Limit() bool {
    return r.current > r.threshold
}

func (r *CustomRegulator) Renormalize() {
    if r.current > 0 {
        r.current -= 0.1 // Gradual reduction
    }
}
```

## Best Practices

1. **Smooth Transitions**: Avoid abrupt state changes that could cause system instability
2. **Metric Selection**: Choose relevant metrics that directly indicate system health
3. **Threshold Tuning**: Set appropriate thresholds based on system capacity and requirements
4. **Composition**: Combine multiple regulators to create comprehensive control systems
5. **State Management**: Maintain clean state management for predictable behavior

## Advanced Topics

### Adaptive Regulation

Regulators can implement adaptive behavior by:
- Learning from historical data
- Adjusting thresholds dynamically
- Predicting future system states

### Hierarchical Regulation

Multiple regulators can be organized hierarchically:
- High-level regulators manage system-wide concerns
- Mid-level regulators handle subsystem regulation
- Low-level regulators control specific components

## Further Reading

- [Circuit Breaker Pattern](https://martinfowler.com/bliki/CircuitBreaker.html)
- [Rate Limiting Patterns](https://cloud.google.com/architecture/rate-limiting-strategies-patterns)
- [Back Pressure in Distributed Systems](https://mechanical-sympathy.blogspot.com/2012/05/apply-back-pressure-when-overloaded.html)
