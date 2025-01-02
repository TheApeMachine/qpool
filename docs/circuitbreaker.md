# Circuit Breaker

The QPool Circuit Breaker implements the circuit breaker pattern to prevent system failures by automatically stopping operations when error rates exceed acceptable thresholds. Like an electrical circuit breaker that trips to prevent damage from power surges, this pattern protects systems from cascade failures.

## Overview

The Circuit Breaker operates in three states:
- **Closed**: Normal operation, requests flow through
- **Open**: Failure threshold exceeded, requests are blocked
- **Half-Open**: Testing recovery, limited requests allowed

## Implementation

```go
type CircuitBreaker struct {
    mu               sync.RWMutex
    maxFailures      int           // Maximum failures before opening
    resetTimeout     time.Duration // Time before recovery attempt
    halfOpenMax      int          // Maximum requests in half-open state
    failureCount     int          // Current consecutive failures
    state            CircuitState  // Current circuit state
    openTime         time.Time     // When circuit was opened
    halfOpenAttempts int          // Attempts in half-open state
    metrics          *Metrics     // System metrics
}
```

### Basic Usage

Create and use a circuit breaker:

```go
// Create breaker: 5 failures max, 1 minute timeout, 3 half-open attempts
breaker := qpool.NewCircuitBreaker(5, time.Minute, 3)

// Use the breaker
if breaker.Allow() {
    err := doRiskyOperation()
    if err != nil {
        breaker.RecordFailure()
    } else {
        breaker.RecordSuccess()
    }
}
```

### State Transitions

The circuit breaker automatically manages state transitions:

```go
// Normal operation (Closed State)
breaker.Allow()        // Returns true
breaker.RecordFailure() // Increment failure count
// After max failures...
breaker.Allow()        // Returns false (Open State)

// After reset timeout...
breaker.Allow()        // Returns true (Half-Open State)
breaker.RecordSuccess() // Move towards Closed State
```

## Features

### Failure Detection

The circuit breaker tracks failures and opens when threshold is exceeded:

```go
func callExternalService() error {
    if !breaker.Allow() {
        return ErrCircuitOpen
    }
    
    if err := externalCall(); err != nil {
        breaker.RecordFailure()
        return err
    }
    
    breaker.RecordSuccess()
    return nil
}
```

### Automatic Recovery

The circuit breaker automatically tests recovery:

```go
// Configure recovery behavior
breaker := qpool.NewCircuitBreaker(
    5,              // Max failures
    time.Minute,    // Reset timeout
    3,              // Half-open max attempts
)

// Recovery happens automatically after timeout
go func() {
    ticker := time.NewTicker(time.Second)
    for range ticker.C {
        breaker.Renormalize()
    }
}()
```

### Adaptive Behavior

The circuit breaker can adjust based on system metrics:

```go
metrics := &Metrics{
    ErrorRate: 0.05,
    Latency:   100 * time.Millisecond,
}
breaker.Observe(metrics)
```

## Best Practices

1. **Choose Appropriate Thresholds**:
   - Consider normal error rates
   - Account for transient failures
   - Set reasonable timeouts

```go
// Example: API with occasional failures
breaker := qpool.NewCircuitBreaker(
    10,             // Allow more failures
    30*time.Second, // Quick recovery
    5,              // Multiple recovery attempts
)
```

2. **Handle Circuit Open State**:
   - Provide fallback behavior
   - Clear error messages
   - Consider partial functionality

```go
func handleRequest() (any, error) {
    if !breaker.Allow() {
        return fallbackBehavior()
    }
    return normalOperation()
}
```

3. **Monitor Circuit State**:
   - Log state transitions
   - Track error patterns
   - Alert on repeated failures

```go
// Example monitoring
type MonitoredBreaker struct {
    *CircuitBreaker
    metrics *Metrics
}

func (mb *MonitoredBreaker) RecordFailure() {
    mb.CircuitBreaker.RecordFailure()
    mb.metrics.IncFailureCount()
}
```

## Use Cases

### External Service Calls

Protect against external service failures:

```go
func callAPI() error {
    if !breaker.Allow() {
        return useCache() // Fallback to cache
    }
    
    resp, err := http.Get(apiURL)
    if err != nil {
        breaker.RecordFailure()
        return err
    }
    
    breaker.RecordSuccess()
    return processResponse(resp)
}
```

### Database Operations

Prevent database overload:

```go
func queryDatabase() (*Result, error) {
    if !breaker.Allow() {
        return nil, ErrDatabaseUnavailable
    }
    
    result, err := db.Query(query)
    if err != nil {
        breaker.RecordFailure()
        return nil, err
    }
    
    breaker.RecordSuccess()
    return result, nil
}
```

### Microservice Communication

Protect microservice dependencies:

```go
func callMicroservice(ctx context.Context) error {
    if !breaker.Allow() {
        return handleDegradedMode(ctx)
    }
    
    if err := grpc.Call(ctx); err != nil {
        breaker.RecordFailure()
        return err
    }
    
    breaker.RecordSuccess()
    return nil
}
```

## Advanced Topics

### Custom Failure Criteria

Define specific conditions for failure:

```go
type SmartBreaker struct {
    CircuitBreaker
    failurePredicate func(error) bool
}

func (sb *SmartBreaker) RecordError(err error) {
    if sb.failurePredicate(err) {
        sb.RecordFailure()
    }
}
```

### Circuit Breaker Groups

Manage multiple related services:

```go
type BreakerGroup struct {
    breakers map[string]*CircuitBreaker
    mu       sync.RWMutex
}

func (bg *BreakerGroup) Allow(service string) bool {
    bg.mu.RLock()
    defer bg.mu.RUnlock()
    return bg.breakers[service].Allow()
}
```

## Further Reading

- [Circuit Breaker Pattern](https://martinfowler.com/bliki/CircuitBreaker.html)
- [Release It!](https://pragprog.com/titles/mnee2/release-it-second-edition/) by Michael Nygard
- [Microsoft Cloud Design Patterns](https://docs.microsoft.com/en-us/azure/architecture/patterns/circuit-breaker)
