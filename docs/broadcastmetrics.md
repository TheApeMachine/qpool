# BroadcastGroup Metrics and Monitoring

## Overview

The BroadcastGroup includes comprehensive metrics and monitoring capabilities to track quantum-aware message distribution, system performance, and resource utilization.

## Metrics System

### Core Metrics

```go
type BroadcastMetrics struct {
    // Message handling
    MessagesSent        int64
    MessagesDropped     int64
    AverageLatency     time.Duration
    
    // System state
    ActiveSubscribers   int
    UncertaintyLevel   UncertaintyLevel
    LastBroadcastTime  time.Time
}
```

## Monitoring Examples

### Basic Metrics Collection

```go
func monitorBroadcastGroup(group *BroadcastGroup) {
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()

    for range ticker.C {
        metrics := group.GetMetrics()
        
        // Log key metrics
        log.Printf("Messages: sent=%d dropped=%d",
            metrics.MessagesSent,
            metrics.MessagesDropped)
            
        log.Printf("Latency: %v, Uncertainty: %v",
            metrics.AverageLatency,
            metrics.UncertaintyLevel)
    }
}
```

### Advanced Monitoring

```go
func advancedMonitoring(group *BroadcastGroup) {
    // Set up monitoring thresholds
    const (
        maxLatency        = 100 * time.Millisecond
        maxUncertainty    = 0.7
        dropRateThreshold = 0.1
    )

    // Monitor with alerting
    go func() {
        for {
            metrics := group.GetMetrics()
            
            // Check message drop rate
            dropRate := float64(metrics.MessagesDropped) /
                       float64(metrics.MessagesSent + metrics.MessagesDropped)
            
            if dropRate > dropRateThreshold {
                alertHighDropRate(dropRate)
            }

            // Check latency
            if metrics.AverageLatency > maxLatency {
                alertHighLatency(metrics.AverageLatency)
            }

            // Check uncertainty
            if metrics.UncertaintyLevel > maxUncertainty {
                alertHighUncertainty(metrics.UncertaintyLevel)
            }

            time.Sleep(time.Second)
        }
    }()
}
```

### Resource Usage Monitoring

```go
func monitorResourceUsage(group *BroadcastGroup) {
    // Monitor subscriber health
    go func() {
        for {
            metrics := group.GetMetrics()
            
            // Check subscriber count
            if metrics.ActiveSubscribers == 0 {
                alertNoSubscribers()
            }

            // Check subscription buffer usage
            for subID, ch := range group.subscribers {
                bufferUsage := float64(len(ch)) / float64(cap(ch))
                if bufferUsage > 0.8 {
                    alertHighBufferUsage(subID, bufferUsage)
                }
            }

            time.Sleep(5 * time.Second)
        }
    }()
}
```

## Metric Types and Usage

### Performance Metrics

- **MessagesSent**: Total successfully delivered messages
- **MessagesDropped**: Messages that couldn't be delivered
- **AverageLatency**: Mean time from Send() to delivery

### State Metrics

- **ActiveSubscribers**: Current number of subscribers
- **UncertaintyLevel**: Current quantum uncertainty
- **LastBroadcastTime**: Timestamp of last broadcast

### Derived Metrics

```go
// Calculate message success rate
func calculateSuccessRate(metrics *BroadcastMetrics) float64 {
    total := metrics.MessagesSent + metrics.MessagesDropped
    if total == 0 {
        return 1.0
    }
    return float64(metrics.MessagesSent) / float64(total)
}

// Calculate health score
func calculateHealthScore(metrics *BroadcastMetrics) float64 {
    successRate := calculateSuccessRate(metrics)
    uncertaintyFactor := 1.0 - float64(metrics.UncertaintyLevel)
    latencyFactor := 1.0 - math.Min(
        1.0,
        float64(metrics.AverageLatency) / float64(time.Second),
    )
    
    return (successRate + uncertaintyFactor + latencyFactor) / 3.0
}
```

## Best Practices

### Monitoring Setup

1. **Regular Intervals**: Check metrics at appropriate intervals:
   - High-frequency: Latency, message rates (1s)
   - Medium-frequency: Uncertainty, subscriber health (5s)
   - Low-frequency: Overall health score (30s)

2. **Alerting Thresholds**: Set up graduated alerting:
   ```go
   const (
       WarningUncertainty  = 0.6
       CriticalUncertainty = 0.8
       WarningLatency     = 50 * time.Millisecond
       CriticalLatency    = 100 * time.Millisecond
   )
   ```

3. **Resource Monitoring**: Watch for resource exhaustion:
   - Channel buffer usage
   - Subscriber count changes
   - Message drop rates

### Metric Collection

1. **Atomic Updates**: Use atomic operations for counter updates
2. **Bounded Collection**: Limit metric history storage
3. **Efficient Sampling**: Use appropriate sampling rates

## Limitations

- Metrics are in-memory only
- No persistent metric history
- Local monitoring only
- Simple statistical aggregation
