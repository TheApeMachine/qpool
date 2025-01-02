# QPool - Quantum Worker Pool & Message Queue

[![Go Reference](https://pkg.go.dev/badge/github.com/theapemachine/qpool.svg)](https://pkg.go.dev/github.com/theapemachine/qpool)
[![Go Report Card](https://goreportcard.com/badge/github.com/theapemachine/qpool)](https://goreportcard.com/report/github.com/theapemachine/qpool)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

QPool is a high-performance, feature-rich worker pool implementation in Go that combines the capabilities of a traditional worker pool with a sophisticated message queue. It's designed to handle complex job dependencies, provide robust error handling, and scale automatically based on workload.

## 🌟 Key Features

-   **Dynamic Worker Pool**

    -   Auto-scaling based on workload
    -   Configurable min/max worker counts
    -   Efficient worker management
    -   Smart job distribution

-   **Advanced Job Dependencies**

    -   Support for future dependencies
    -   Dependency chain resolution
    -   Circular dependency detection
    -   Parent-child relationship tracking

-   **Robust Error Handling**

    -   Circuit breaker pattern
    -   Configurable retry policies
    -   Exponential backoff
    -   Timeout management

-   **Performance Features**

    -   Non-blocking job scheduling
    -   Efficient memory usage
    -   Resource utilization tracking
    -   Load-based auto-scaling

-   **Monitoring & Metrics**
    -   Comprehensive metrics collection
    -   Latency percentiles (p95, p99)
    -   Success/failure rates
    -   Resource utilization stats
    -   Dependency resolution tracking

## ⚛️ Quantum Entanglement

QPool introduces the concept of Entanglement, inspired by quantum mechanics. Just as quantum particles can be entangled so that the state of one instantly affects the other, jobs in an Entanglement share state that persists across time and space. When one job updates the shared state, all other jobs in the entanglement—even those not yet processed—will see these changes.

This powerful feature enables:
- Shared state between related jobs that persists even across job completion
- Automatic state synchronization for jobs processed at different times
- Immutable history of all state changes
- Perfect for distributed data processing and coordinated task execution

[Learn more about Entanglement with code samples →](docs/entanglement.md)

## 📦️ Regulators

QPool implements a sophisticated regulation system inspired by biological and mechanical control systems. Like a thermostat regulating temperature or a governor controlling engine speed, Regulators maintain system stability and prevent resource exhaustion through adaptive control mechanisms.

The regulation system includes:

### 🚦 Rate Limiter

A token bucket-based rate limiter that provides smooth, burst-capable traffic control. Like a water tank with controlled inflow and outflow, it ensures operations proceed at a sustainable pace while allowing brief bursts of activity when needed.

- Configurable steady-state rate and burst capacity
- Smooth operation without sharp cutoffs
- Perfect for API rate limiting and resource protection
- Automatic token replenishment

[Learn more about Rate Limiting →](docs/ratelimiter.md)

### 🔌 Circuit Breaker

Inspired by electrical circuit breakers, this pattern prevents system failure by automatically stopping operations when error rates exceed acceptable thresholds. Like its electrical counterpart, it "trips" to protect the system and automatically tests for recovery.

- Prevents cascade failures in distributed systems
- Self-healing with automatic recovery testing
- Configurable error thresholds and recovery timing
- Perfect for protecting external service calls

[Learn more about Circuit Breakers →](docs/circuitbreaker.md)

### ⚖️ Load Balancer

Like a traffic controller directing vehicles to different lanes based on congestion, the Load Balancer ensures optimal work distribution across workers by considering their capacity, current load, and performance characteristics.

- Intelligent work distribution based on metrics
- Automatic worker selection and load tracking
- Adapts to worker pool scaling
- Perfect for optimizing resource utilization

[Learn more about Load Balancing →](docs/loadbalancer.md)

[Learn more about the Regulation System →](docs/regulator.md)

## 📦 Installation

```bash
go get github.com/theapemachine/qpool
```

## 🚀 Quick Start

Here's a simple example to get you started:

```go
package main

import (
    "context"
    "time"
    "github.com/theapemachine/qpool"
)

func main() {
    // Create a new pool with min 2, max 5 workers
    ctx := context.Background()
    pool := qpool.NewQ(ctx, 2, 5, &qpool.Config{
        SchedulingTimeout: time.Second,
    })
    defer pool.Close()

    // Schedule a simple job
    result := pool.Schedule("job-1", func() (any, error) {
        return "Hello, World!", nil
    })

    // Wait for the result
    value := <-result
    if value.Error != nil {
        panic(value.Error)
    }
    println(value.Value.(string))
}
```

## 🔨 Advanced Usage

### Job Dependencies

QPool supports a robust job dependency system that allows you to create complex workflows. Jobs can depend on one or more other jobs, and the system ensures proper execution order.

```go
// Create jobs with dependencies
job1Result := pool.Schedule("data-fetch", func() (any, error) {
    return fetchData()
}, qpool.WithTTL(time.Minute))

// This job will only execute after data-fetch completes successfully
job2Result := pool.Schedule("data-process", func() (any, error) {
    return processData()
}, qpool.WithDependencies([]string{"data-fetch"}))

// You can also add multiple dependencies
job3Result := pool.Schedule("data-aggregate", func() (any, error) {
    return aggregateData()
}, qpool.WithDependencies([]string{"data-fetch", "data-process"}))

// Configure dependency retry behavior
job4Result := pool.Schedule("data-transform", func() (any, error) {
    return transformData()
}, 
    qpool.WithDependencies([]string{"data-aggregate"}),
    qpool.WithDependencyRetry(3, &qpool.ExponentialBackoff{Initial: time.Second}))

// Process results
for result := range job4Result {
    if result.Error != nil {
        log.Printf("Error: %v", result.Error)
        continue
    }
    // Process the result
    log.Printf("Success: %v", result.Value)
}
```

Key features of the dependency system:
- Jobs wait for all dependencies to complete successfully before starting
- If any dependency fails, dependent jobs fail automatically
- Configurable retry policies for dependency resolution
- Automatic cleanup of completed job results based on TTL
- Non-blocking dependency resolution with timeout handling

### Circuit Breaker

```go
// Add circuit breaker to protect sensitive operations
pool.Schedule("api-call", func() (any, error) {
    return callExternalAPI()
}, qpool.WithCircuitBreaker("api", 5, time.Minute))
```

### Retry Policy

```go
// Configure retry behavior
pool.Schedule("flaky-operation", func() (any, error) {
    return flakyOperation()
}, qpool.WithRetry(3, &qpool.ExponentialBackoff{
    Initial: time.Second,
}))
```

### Broadcast Groups

```go
// Create a broadcast group for pub/sub functionality
group := pool.CreateBroadcastGroup("sensors", time.Minute)
subscriber := pool.Subscribe("sensors")

// Send updates to all subscribers
group.Send(qpool.QValue{
    Value: "sensor-update",
    CreatedAt: time.Now(),
})
```

## 📊 Metrics & Monitoring

QPool provides comprehensive metrics for monitoring:

```go
// Get current metrics
metrics := pool.metrics.ExportMetrics()

fmt.Printf("Active Workers: %d\n", metrics["worker_count"])
fmt.Printf("Queue Size: %d\n", metrics["queue_size"])
fmt.Printf("Success Rate: %.2f%%\n", metrics["success_rate"]*100)
fmt.Printf("P95 Latency: %dms\n", metrics["p95_latency"])
```

## 🔧 Configuration

QPool can be configured through the `Config` struct:

```go
config := &qpool.Config{
    SchedulingTimeout: time.Second * 5,
}

pool := qpool.NewQ(ctx, minWorkers, maxWorkers, config)
```

## 🏗️ Architecture

QPool consists of several key components:

-   **Q (Pool)**: Main orchestrator managing workers and job scheduling
-   **Worker**: Handles job execution and resource management
-   **QuantumSpace**: Manages job results and dependencies
-   **CircuitBreaker**: Provides fault tolerance
-   **Scaler**: Handles dynamic worker pool sizing
-   **Metrics**: Collects and exposes performance data

## 📈 Performance

QPool is designed for high performance:

-   Non-blocking job scheduling
-   Efficient memory usage
-   Smart resource allocation
-   Automatic scaling based on load
-   Optimized dependency resolution

## 🧪 Testing

Run the test suite:

```bash
go test -v ./...
```

Run with race detection:

```bash
go test -race -v ./...
```

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

-   Inspired by a conversation with Clause AI
-   Built with modern concurrency patterns and best practices
-   Designed for real-world production use cases

## 📚 Documentation

For detailed documentation, please visit our [Go Docs](https://pkg.go.dev/github.com/theapemachine/qpool).

## 📞 Support

-   Create an issue for bug reports
-   Start a discussion for feature requests
-   Check existing issues before creating new ones

---

Made with ❤️ by Daniel Owen van Dommelen

## Regulators

QPool implements a biological-inspired regulation system that helps maintain system stability and optimal performance under varying loads. Like a living organism that regulates its internal state through multiple coordinated mechanisms, QPool uses a set of regulators to control different aspects of the worker pool:

- **RateLimiter**: Implements the token bucket algorithm to provide smooth, predictable rate limiting with burst capacity. Perfect for API rate limiting or controlling resource consumption rates. [Learn more](docs/ratelimiter.md)

- **CircuitBreaker**: Prevents system failures by stopping operations when error rates exceed acceptable thresholds. Features self-healing capabilities through gradual recovery mechanisms. [Learn more](docs/circuitbreaker.md)

- **LoadBalancer**: Distributes work evenly across workers while making intelligent routing decisions based on worker performance metrics and current load. [Learn more](docs/loadbalancer.md)

- **BackPressureRegulator**: Manages system pressure by controlling job intake based on queue size and processing times, preventing system overload. [Learn more](docs/backpressure.md)

- **ResourceGovernorRegulator**: Monitors and manages system resource usage (CPU, memory) to prevent resource exhaustion and maintain system stability. [Learn more](docs/resourcegovernor.md)

- **AdaptiveScalerRegulator**: Dynamically adjusts the worker pool size based on load metrics and performance data to optimize resource utilization. [Learn more](docs/adaptivescaler.md)

All regulators implement the `Regulator` interface, allowing them to be composed and combined to create sophisticated control systems. [Learn more about the Regulator interface](docs/regulator.md)
