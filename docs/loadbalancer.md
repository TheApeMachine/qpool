# Load Balancer

The QPool Load Balancer implements intelligent work distribution across workers while implementing the Regulator interface. Like a traffic controller directing vehicles to different lanes based on congestion, it ensures optimal work distribution by considering worker capacity, current load, and performance metrics.

## Overview

The Load Balancer maintains detailed statistics about each worker:
- Current load (number of active jobs)
- Processing latency (average job completion time)
- Maximum capacity (concurrent job limit)
- Performance metrics (through the Regulator interface)

## Implementation

```go
type LoadBalancer struct {
    workerLoads    map[int]float64       // Current load per worker
    workerLatency  map[int]time.Duration // Average processing time per worker
    workerCapacity map[int]int          // Maximum concurrent jobs per worker
    activeWorkers  int                  // Number of available workers
    metrics        *Metrics             // System metrics
}
```

### Basic Usage

Create and use a load balancer:

```go
// Create balancer with 5 workers, each handling up to 10 concurrent jobs
balancer := qpool.NewLoadBalancer(5, 10)

// Get next available worker
workerID, err := balancer.SelectWorker()
if err != nil {
    // Handle no available workers
    return err
}

// Record job lifecycle
balancer.RecordJobStart(workerID)
result := processJob()
balancer.RecordJobComplete(workerID, jobDuration)
```

### Advanced Configuration

Configure the load balancer for different scenarios:

```go
// Microservices with varying capacities
microserviceBalancer := qpool.NewLoadBalancer(
    10,  // 10 service instances
    100, // 100 concurrent requests each
)

// Batch processing with high throughput
batchBalancer := qpool.NewLoadBalancer(
    3,    // 3 batch processors
    1000, // 1000 concurrent jobs each
)

// API gateway routing
apiBalancer := qpool.NewLoadBalancer(
    5,   // 5 API servers
    50,  // 50 concurrent requests each
)
```

## Features

### Intelligent Worker Selection

The load balancer chooses workers based on multiple factors:

```go
// Worker selection considers:
// - Current load
// - Historical performance
// - Processing latency
workerID, err := balancer.SelectWorker()
if err == nil {
    assignJobToWorker(workerID, job)
}
```

### Load Tracking

Accurate load tracking through job lifecycle:

```go
func processWithLoadTracking(job Job, workerID int) error {
    balancer.RecordJobStart(workerID)
    startTime := time.Now()
    
    err := processJob(job)
    
    balancer.RecordJobComplete(workerID, time.Since(startTime))
    return err
}
```

### Adaptive Behavior

The load balancer adjusts to system conditions:

```go
// System metrics affect routing decisions
metrics := &Metrics{
    WorkerCount: 5,
    QueueDepth:  100,
    CPUUsage:    0.8,
}
balancer.Observe(metrics)
```

## Best Practices

1. **Choose Appropriate Capacities**:
   - Consider worker resources
   - Account for job complexity
   - Leave headroom for spikes

```go
// Example: CPU-intensive workers
balancer := qpool.NewLoadBalancer(
    runtime.NumCPU(), // One worker per CPU
    2,                // 2 concurrent jobs per worker
)
```

2. **Monitor Worker Health**:
   - Track error rates
   - Watch processing times
   - Detect stuck workers

```go
type MonitoredBalancer struct {
    *LoadBalancer
    healthChecks map[int]time.Time
}

func (mb *MonitoredBalancer) checkHealth() {
    for workerID, lastSeen := range mb.healthChecks {
        if time.Since(lastSeen) > threshold {
            alertUnhealthyWorker(workerID)
        }
    }
}
```

3. **Handle Edge Cases**:
   - Worker unavailability
   - Capacity exhaustion
   - Performance degradation

```go
func assignJob(job Job) error {
    workerID, err := balancer.SelectWorker()
    if err == ErrNoAvailableWorkers {
        return handleBackPressure(job)
    }
    return assignToWorker(workerID, job)
}
```

## Use Cases

### Microservice Load Balancing

Distribute requests across service instances:

```go
func handleRequest(w http.ResponseWriter, r *http.Request) {
    workerID, err := balancer.SelectWorker()
    if err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        return
    }
    
    forwardToInstance(workerID, w, r)
}
```

### Batch Job Processing

Distribute batch processing tasks:

```go
func processBatch(items []Item) error {
    for _, item := range items {
        workerID, err := balancer.SelectWorker()
        if err != nil {
            return err
        }
        
        go func(item Item, worker int) {
            balancer.RecordJobStart(worker)
            processItem(item)
            balancer.RecordJobComplete(worker, time.Since(start))
        }(item, workerID)
    }
    return nil
}
```

### Dynamic Worker Pools

Handle worker pool scaling:

```go
func scaleWorkerPool(newSize int) {
    metrics := &Metrics{
        WorkerCount: newSize,
    }
    balancer.Observe(metrics) // Balancer adapts to new worker count
}
```

## Advanced Topics

### Custom Selection Strategies

Implement specialized worker selection:

```go
type PriorityBalancer struct {
    LoadBalancer
    workerPriority map[int]int
}

func (pb *PriorityBalancer) SelectWorker() (int, error) {
    // Consider worker priority in selection
    return pb.selectByPriority()
}
```

### Locality-Aware Balancing

Consider worker location in routing:

```go
type GeoBalancer struct {
    LoadBalancer
    workerRegions map[int]string
}

func (gb *GeoBalancer) SelectWorker(clientRegion string) (int, error) {
    return gb.selectInRegion(clientRegion)
}
```

## Further Reading

- [Load Balancing Algorithms](https://www.nginx.com/resources/glossary/load-balancing/)
- [Distributed Systems Design](https://aws.amazon.com/builders-library/workload-isolation-using-shuffle-sharding/)
- [Performance Patterns](https://docs.microsoft.com/en-us/azure/architecture/patterns/category/performance-scalability) 