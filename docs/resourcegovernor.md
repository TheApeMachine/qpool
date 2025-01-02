# Resource Governor Regulator

The ResourceGovernorRegulator is a sophisticated control mechanism that monitors and manages system resource usage (CPU and memory) to prevent resource exhaustion and maintain system stability.

## Overview

Like a power management system in a modern device, the ResourceGovernorRegulator continuously monitors system resource utilization and takes action to prevent overload conditions. It tracks both CPU and memory usage, applying limits when either resource approaches configured thresholds.

## Features

- Real-time CPU and memory usage monitoring
- Configurable thresholds for both CPU and memory
- Automatic resource usage tracking
- Gradual recovery mechanism
- Thread-safe operation

## Usage

### Creating a Resource Governor

```go
// Create a new resource governor with:
// - 80% max CPU usage
// - 90% max memory usage
// - Check resources every second
governor := NewResourceGovernorRegulator(0.8, 0.9, time.Second)
```

### Basic Operation

```go
// Observe current metrics
governor.Observe(metrics)

// Check if we should limit operations
if governor.Limit() {
    // Resource usage is too high, delay or reject operation
    return ErrResourceLimited
}

// Proceed with operation
doWork()
```

### Getting Resource Usage

```go
// Get current CPU and memory usage
cpu, mem := governor.GetResourceUsage()
fmt.Printf("CPU: %.2f%%, Memory: %.2f%%\n", cpu*100, mem*100)

// Get configured thresholds
maxCPU, maxMem := governor.GetThresholds()
fmt.Printf("Max CPU: %.2f%%, Max Memory: %.2f%%\n", maxCPU*100, maxMem*100)
```

## Configuration Parameters

- `maxCPUPercent`: Maximum allowed CPU usage (0.0 to 1.0)
- `maxMemoryPercent`: Maximum allowed memory usage (0.0 to 1.0)
- `checkInterval`: How often to update resource measurements

## How It Works

1. **Resource Monitoring**
   - CPU usage is tracked through metrics
   - Memory usage is obtained via runtime statistics
   - Both metrics are updated on each observation

2. **Limitation Logic**
   - Operations are limited if either:
     - CPU usage exceeds maxCPUPercent
     - Memory usage exceeds maxMemoryPercent
   - Limits remain in place until resource usage drops

3. **Renormalization**
   - Periodically updates resource measurements
   - Allows operations to resume when resource usage drops
   - Prevents rapid oscillation through gradual recovery

## Best Practices

1. **Threshold Selection**
   - Set CPU threshold below 100% to leave headroom
   - Set memory threshold to prevent swapping
   - Consider system-specific requirements

2. **Monitoring Interval**
   - Balance responsiveness with overhead
   - Typical values range from 100ms to 1s
   - Adjust based on application needs

3. **Resource Management**
   - Combine with other regulators for comprehensive control
   - Consider implementing graceful degradation
   - Monitor and log resource usage patterns

## Example Scenarios

### High CPU Usage

```go
governor := NewResourceGovernorRegulator(0.8, 0.9, time.Second)

// CPU-intensive operation
for {
    if governor.Limit() {
        time.Sleep(100 * time.Millisecond) // Back off
        continue
    }
    performCPUIntensiveTask()
}
```

### Memory Management

```go
governor := NewResourceGovernorRegulator(0.8, 0.85, time.Second)

// Memory-intensive operation
func processLargeData(data []byte) error {
    if governor.Limit() {
        return ErrResourceLimited
    }
    return processData(data)
}
```

## Integration with QPool

```go
pool := qpool.NewQ(ctx, minWorkers, maxWorkers, &qpool.Config{
    Regulators: []qpool.Regulator{
        NewResourceGovernorRegulator(0.8, 0.9, time.Second),
    },
})
```

## Advanced Usage

### Custom Resource Monitoring

```go
type CustomResourceMonitor struct {
    *ResourceGovernorRegulator
    customMetrics chan float64
}

func (m *CustomResourceMonitor) Observe(metrics *Metrics) {
    // Add custom resource monitoring
    select {
    case usage := <-m.customMetrics:
        metrics.ResourceUtilization = usage
    default:
    }
    m.ResourceGovernorRegulator.Observe(metrics)
}
```

### Adaptive Thresholds

```go
type AdaptiveResourceGovernor struct {
    *ResourceGovernorRegulator
    history []float64
}

func (g *AdaptiveResourceGovernor) adjustThresholds() {
    // Adjust thresholds based on historical usage patterns
    avg := calculateMovingAverage(g.history)
    g.maxCPUPercent = min(0.9, avg*1.2) // 20% headroom
}
```

## Troubleshooting

1. **High Rejection Rate**
   - Check if thresholds are too conservative
   - Monitor actual resource usage patterns
   - Consider gradual threshold adjustment

2. **Resource Leaks**
   - Verify proper cleanup in worker tasks
   - Monitor long-term resource usage trends
   - Implement resource usage logging

3. **Performance Impact**
   - Adjust check interval if overhead is high
   - Profile resource monitoring impact
   - Consider batch processing for heavy workloads

## Further Reading

- [Go Runtime Statistics](https://golang.org/pkg/runtime/#MemStats)
- [System Resource Management](https://www.kernel.org/doc/Documentation/cgroup-v1/memory.txt)
- [CPU Throttling Strategies](https://en.wikipedia.org/wiki/CPU_throttling) 