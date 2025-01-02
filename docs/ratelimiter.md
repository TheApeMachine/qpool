# Rate Limiter

The QPool Rate Limiter implements the token bucket algorithm to provide smooth, predictable rate limiting with burst capacity. Like a water tank with controlled inflow and outflow, it ensures operations proceed at a sustainable pace while allowing brief bursts of activity when needed.

## Overview

The Rate Limiter maintains a bucket of tokens that are:
- Consumed when operations are performed
- Replenished at a fixed rate over time
- Capped at a maximum capacity (burst limit)

## Implementation

```go
type RateLimiter struct {
    tokens     int           // Current available tokens
    maxTokens  int           // Maximum token capacity
    refillRate time.Duration // Time between token replenishments
    lastRefill time.Time     // Last token replenishment time
    mu         sync.Mutex    // Thread safety
    metrics    *Metrics      // System metrics
}
```

### Basic Usage

Create a rate limiter with desired parameters:

```go
// Allow 100 operations per second with burst capacity of 100
limiter := qpool.NewRateLimiter(100, time.Second)

// Check if operation should proceed
if !limiter.Limit() {
    // Perform operation
} else {
    // Handle rate limit
}
```

### Advanced Configuration

Configure the rate limiter for specific use cases:

```go
// API rate limiting (10 req/sec, max burst of 20)
apiLimiter := qpool.NewRateLimiter(20, 100*time.Millisecond)

// Database connection limiting (1000 conn/min, max burst of 100)
dbLimiter := qpool.NewRateLimiter(100, 60*time.Millisecond)

// Batch job limiting (10000 jobs/hour, max burst of 1000)
batchLimiter := qpool.NewRateLimiter(1000, 360*time.Millisecond)
```

## Features

### Smooth Rate Limiting

The token bucket algorithm provides smooth rate limiting without sharp cutoffs:

```go
// Tokens are consumed gradually
for i := 0; i < 1000; i++ {
    if !limiter.Limit() {
        processItem(i)
    } else {
        // Wait for token replenishment
        time.Sleep(time.Millisecond * 100)
    }
}
```

### Burst Handling

The limiter allows brief bursts of activity up to the maximum token capacity:

```go
// Configure with burst capacity
limiter := qpool.NewRateLimiter(
    100,        // Max burst
    time.Second // Replenish 1 token/second
)

// Burst of operations can proceed immediately
for i := 0; i < 100; i++ {
    if !limiter.Limit() {
        processImmediately(i)
    }
}
```

### Adaptive Behavior

The rate limiter can adjust its behavior based on system metrics:

```go
// System metrics affect rate limiting
metrics := &Metrics{
    QueueDepth: 100,
    CPUUsage:   0.8,
}
limiter.Observe(metrics)
```

## Best Practices

1. **Choose Appropriate Rates**:
   - Consider system capacity
   - Account for downstream service limits
   - Leave headroom for spikes

```go
// Example: API with 1000 req/sec capacity
limiter := qpool.NewRateLimiter(
    800,         // 80% of capacity
    time.Second,
)
```

2. **Handle Rate Limiting Gracefully**:
   - Implement backoff strategies
   - Provide clear feedback
   - Consider retry mechanisms

```go
func handleRequest() error {
    if limiter.Limit() {
        return &RateLimitError{
            RetryAfter: time.Second,
        }
    }
    return processRequest()
}
```

3. **Monitor and Adjust**:
   - Track rate limit hits
   - Adjust limits based on usage patterns
   - Consider time-of-day variations

```go
// Example monitoring
metrics := &Metrics{
    RateLimitHits: rateLimitCounter,
    TimeOfDay:     time.Now().Hour(),
}
limiter.Observe(metrics)
```

## Use Cases

### API Rate Limiting

Protect APIs from excessive use:

```go
func handleAPIRequest(w http.ResponseWriter, r *http.Request) {
    if limiter.Limit() {
        w.WriteHeader(http.StatusTooManyRequests)
        return
    }
    processAPI(w, r)
}
```

### Resource Protection

Prevent resource exhaustion:

```go
func acquireConnection(pool *ConnectionPool) (*Connection, error) {
    if limiter.Limit() {
        return nil, ErrNoAvailableConnections
    }
    return pool.Get()
}
```

### Batch Processing

Control batch job processing rates:

```go
func processBatch(items []Item) {
    for _, item := range items {
        for limiter.Limit() {
            time.Sleep(time.Millisecond * 100)
        }
        processItem(item)
    }
}
```

## Advanced Topics

### Distributed Rate Limiting

For distributed systems, consider using a shared token bucket:

```go
type DistributedRateLimiter struct {
    RateLimiter
    redis *Redis
}

func (d *DistributedRateLimiter) Limit() bool {
    return d.redis.Eval(tokenBucketScript)
}
```

### Multiple Rate Limits

Combine multiple limiters for different aspects:

```go
type MultiLimiter struct {
    requestLimiter *RateLimiter
    dateLimiter   *RateLimiter
    userLimiter   *RateLimiter
}

func (m *MultiLimiter) Limit() bool {
    return m.requestLimiter.Limit() ||
           m.dateLimiter.Limit() ||
           m.userLimiter.Limit()
}
```

## Further Reading

- [Token Bucket Algorithm](https://en.wikipedia.org/wiki/Token_bucket)
- [Rate Limiting Patterns](https://cloud.google.com/architecture/rate-limiting-strategies-patterns)
- [System Design: Rate Limiting](https://medium.com/@saisandeepmopuri/system-design-rate-limiter-and-data-modelling-9304b0d18250)
