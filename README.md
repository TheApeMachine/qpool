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

```go
// Create jobs with dependencies
pool.Schedule("data-fetch", func() (any, error) {
    return fetchData()
}, qpool.WithTTL(time.Minute))

pool.Schedule("data-process", func() (any, error) {
    return processData()
}, qpool.WithDependencies([]string{"data-fetch"}))
```

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
group.Send(qpool.QuantumValue{
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
