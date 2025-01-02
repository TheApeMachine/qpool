# Back Pressure Regulator

The BackPressureRegulator is a sophisticated flow control mechanism that helps maintain system stability by managing the rate of incoming work based on the system's current processing capacity and queue depth.

## Overview

Like a pressure relief valve in a physical system, the BackPressureRegulator prevents system overload by creating resistance to incoming work when the system is under stress. It uses a combination of queue size and processing time metrics to determine the current "pressure" level and adjust work intake accordingly.

## Features

- Dynamic pressure calculation based on multiple metrics
- Configurable queue size limits
- Processing time monitoring
- Gradual pressure relief mechanism
- Weighted metric combination

## Usage

### Creating a Back Pressure Regulator

```go
// Create a new back pressure regulator with:
// - Maximum queue size of 1000
// - Target processing time of 1 second
// - Pressure measurement window of 1 minute
regulator := NewBackPressureRegulator(1000, time.Second, time.Minute)
```

### Basic Operation

```go
// Observe current metrics
regulator.Observe(metrics)

// Check if we should apply back pressure
if regulator.Limit() {
    // System is under pressure, delay or reject work
    return ErrBackPressure
}

// Process work normally
processWork()
```

### Getting Pressure Level

```go
// Get current pressure level (0.0 to 1.0)
pressure := regulator.GetPressure()
fmt.Printf("Current system pressure: %.2f%%\n", pressure*100)
```

## Configuration Parameters

- `maxQueueSize`: Maximum allowed queue size
- `targetProcessTime`: Target processing time for jobs
- `pressureWindow`: Time window for pressure measurements

## How It Works

1. **Pressure Calculation**
   - Queue pressure (60% weight):
     - Based on current queue size vs maximum
     - Linear scaling from 0 to max queue size
   - Latency pressure (40% weight):
     - Based on processing time vs target time
     - Exponential scaling for times exceeding target

2. **Limitation Logic**
   - Pressure threshold is 0.8 (80%)
   - Above threshold: Apply back pressure
   - Below threshold: Allow work to proceed

3. **Pressure Relief**
   - Gradual pressure reduction during good conditions
   - Prevents rapid oscillation
   - Maintains system stability

## Best Practices

1. **Queue Size Configuration**
   - Set based on available memory
   - Consider processing rate
   - Leave headroom for bursts

2. **Processing Time Targets**
   - Set realistic targets
   - Account for normal processing variation
   - Consider SLA requirements

3. **Pressure Window**
   - Balance responsiveness with stability
   - Typical values: 30s to 5min
   - Adjust based on workload patterns

## Example Scenarios

### API Rate Control

```go
regulator := NewBackPressureRegulator(
    1000,           // Queue size
    time.Second,    // Target response time
    time.Minute,    // Measurement window
)

func handleRequest(w http.ResponseWriter, r *http.Request) {
    if regulator.Limit() {
        http.Error(w, "Service under high load", http.StatusServiceUnavailable)
        return
    }
    processRequest(w, r)
}
```

### Batch Processing

```go
regulator := NewBackPressureRegulator(
    5000,           // Larger queue for batch jobs
    time.Second*5,  // Longer processing time target
    time.Minute*5,  // Longer measurement window
)

func processBatch(items []Item) error {
    if regulator.Limit() {
        return ErrSystemOverloaded
    }
    return processBatchItems(items)
}
```

## Integration with QPool

```go
pool := qpool.NewQ(ctx, minWorkers, maxWorkers, &qpool.Config{
    Regulators: []qpool.Regulator{
        NewBackPressureRegulator(1000, time.Second, time.Minute),
    },
})
```

## Advanced Usage

### Custom Pressure Calculation

```go
type CustomPressureRegulator struct {
    *BackPressureRegulator
    customFactors []float64
}

func (r *CustomPressureRegulator) updatePressure() {
    // Custom pressure calculation logic
    queuePressure := float64(r.metrics.JobQueueSize) / float64(r.maxQueueSize)
    latencyPressure := float64(r.metrics.AverageJobLatency) / float64(r.targetProcessTime)
    
    // Apply custom weighting factors
    r.currentPressure = (queuePressure * r.customFactors[0]) +
                       (latencyPressure * r.customFactors[1])
}
```

### Adaptive Targets

```go
type AdaptiveBackPressure struct {
    *BackPressureRegulator
    history []time.Duration
}

func (r *AdaptiveBackPressure) adjustTargets() {
    // Adjust target process time based on historical performance
    p95 := calculateP95Latency(r.history)
    r.targetProcessTime = p95 * 1.2 // 20% headroom
}
```

## Troubleshooting

1. **Frequent Back Pressure**
   - Check if queue size limit is too low
   - Verify processing time targets are realistic
   - Consider scaling system capacity

2. **Pressure Oscillation**
   - Increase pressure window
   - Adjust relief rate
   - Review pressure calculation weights

3. **Poor Responsiveness**
   - Decrease pressure window
   - Adjust threshold
   - Monitor system metrics

## Further Reading

- [Back Pressure Pattern](https://www.reactivemanifesto.org/glossary#Back-Pressure)
- [Flow Control in Systems](https://mechanical-sympathy.blogspot.com/2012/05/apply-back-pressure-when-overloaded.html)
- [Queue Theory](https://en.wikipedia.org/wiki/Queueing_theory) 