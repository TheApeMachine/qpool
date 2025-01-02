# Regulation System

The QPool regulation system provides a flexible framework for implementing various control mechanisms that maintain system stability and prevent resource exhaustion. Inspired by control systems in nature and engineering, regulators act as the system's homeostatic mechanisms.

## Overview

Just as biological systems use feedback loops to maintain stability, and mechanical systems use governors to control speed, QPool's regulators monitor and adjust system behavior to maintain optimal performance. Each regulator implements a common interface that provides three key operations:

```go
type Regulator interface {
    Observe(metrics *Metrics)
    Limit() bool
    Renormalize()
}
```

## Core Concepts

### Observation (Sensing)

Like sensors in a control system, regulators observe system metrics through the `Observe()` method. This provides the feedback necessary for making control decisions:

```go
metrics := &Metrics{
    QueueDepth: 100,
    ErrorRate: 0.05,
    Latency: 50 * time.Millisecond,
}
regulator.Observe(metrics)
```

### Limitation (Control)

The `Limit()` method acts as the control point, determining whether operations should proceed or be restricted based on observed conditions:

```go
if regulator.Limit() {
    // Operation should be restricted
    return ErrRateLimited
}
// Proceed with operation
```

### Renormalization (Recovery)

Like a feedback loop seeking equilibrium, `Renormalize()` attempts to restore normal operation after periods of restriction:

```go
// Periodically attempt system recovery
go func() {
    ticker := time.NewTicker(checkInterval)
    for range ticker.C {
        regulator.Renormalize()
    }
}()
```

## Implementation Examples

### Creating Custom Regulators

You can implement custom regulators by satisfying the Regulator interface:

```go
type CustomRegulator struct {
    threshold int
    current   int
    mu        sync.Mutex
}

func (cr *CustomRegulator) Observe(metrics *Metrics) {
    cr.mu.Lock()
    defer cr.mu.Unlock()
    cr.current = metrics.QueueDepth
}

func (cr *CustomRegulator) Limit() bool {
    return cr.current >= cr.threshold
}

func (cr *CustomRegulator) Renormalize() {
    cr.mu.Lock()
    defer cr.mu.Unlock()
    cr.current = 0
}
```

### Combining Regulators

Multiple regulators can be combined for comprehensive system protection:

```go
pool := qpool.NewQ(ctx, minWorkers, maxWorkers, &qpool.Config{
    Regulators: []qpool.Regulator{
        qpool.NewRateLimiter(100, time.Second),
        qpool.NewCircuitBreaker(5, time.Minute, 3),
    },
})
```

## Best Practices

1. **Appropriate Thresholds**: Choose regulation thresholds based on system capacity and requirements.
2. **Gradual Recovery**: Implement gentle recovery mechanisms to prevent oscillation.
3. **Metric Selection**: Monitor metrics that directly indicate system health.
4. **Coordination**: Consider how multiple regulators interact when used together.

## Built-in Regulators

QPool provides two built-in regulators:

- [Rate Limiter](ratelimiter.md): Controls operation rates using a token bucket algorithm
- [Circuit Breaker](circuitbreaker.md): Prevents cascade failures by stopping operations during error conditions

## Use Cases

- **API Rate Limiting**: Protect external services from overload
- **Resource Protection**: Prevent system resource exhaustion
- **Graceful Degradation**: Maintain partial service during high load
- **Error Prevention**: Stop operations when error rates are high
- **Load Shedding**: Reject excess work during peak periods

## Advanced Topics

### Adaptive Regulation

Regulators can adapt their behavior based on system conditions:

```go
func (r *AdaptiveRegulator) Observe(metrics *Metrics) {
    r.threshold = calculateThreshold(metrics)
}
```

### Hierarchical Regulation

Regulators can be organized in hierarchies for complex systems:

```go
type HierarchicalRegulator struct {
    children []Regulator
}

func (hr *HierarchicalRegulator) Limit() bool {
    for _, child := range hr.children {
        if child.Limit() {
            return true
        }
    }
    return false
}
```

## Further Reading

- [Circuit Breaker Pattern](https://martinfowler.com/bliki/CircuitBreaker.html)
- [Rate Limiting Patterns](https://cloud.google.com/architecture/rate-limiting-strategies-patterns)
- [Control Theory](https://en.wikipedia.org/wiki/Control_theory)
