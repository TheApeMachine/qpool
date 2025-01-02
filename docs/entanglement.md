# Quantum Entanglement in QPool

## Overview

The Entanglement type in QPool provides a quantum-inspired way to share state between related jobs. Like quantum entangled particles that maintain their connection regardless of distance or time, jobs in an Entanglement share a state that persists even if the jobs are processed at different times or on different workers.

## Key Concepts

### Immutable State History

Every state change in an Entanglement is recorded in an immutable ledger. This ensures that even jobs that start processing later will see the complete history of state changes, maintaining the quantum-like property of entanglement across time.

### State Synchronization

When a job updates the shared state, all other jobs in the entanglement—whether currently running, queued, or added later—will see these changes. This creates a causally consistent view of the shared state across all related jobs.

### Time-Independent Behavior

Like quantum entanglement that works instantaneously regardless of when the particles are measured, the state changes in an Entanglement affect all jobs regardless of when they are processed.

## Code Examples

### Creating an Entanglement

```go
// Create an entanglement with two related jobs
jobs := []qpool.Job{
    {ID: "data-fetch"},
    {ID: "data-process"},
}
entanglement := qpool.NewEntanglement("data-pipeline", jobs, 1*time.Hour)
```

### Sharing State Between Jobs

```go
// In the first job (data-fetch)
pool.Schedule("data-fetch", func() (any, error) {
    data := fetchData()
    entanglement.UpdateState("raw-data", data)
    return "fetch-complete", nil
})

// In the second job (data-process)
// Even if this job starts after data-fetch is complete,
// it will still see the state changes
pool.Schedule("data-process", func() (any, error) {
    if value, exists := entanglement.GetState("raw-data"); exists {
        processedData := processData(value)
        entanglement.UpdateState("processed-data", processedData)
        return "process-complete", nil
    }
    return nil, errors.New("no data to process")
})
```

### State Change Notifications

```go
// Set up a callback for state changes
entanglement.OnStateChange = func(oldState, newState map[string]any) {
    log.Printf("State changed from %v to %v", oldState, newState)
}

// All state changes will trigger this callback
entanglement.UpdateState("status", "processing")
// Logs: State changed from map[] to map[status:processing]
```

### Accessing State History

```go
// Get complete state history
history := entanglement.GetStateHistory(0)
for _, change := range history {
    fmt.Printf("At %v: %s = %v\n", change.Timestamp, change.Key, change.Value)
}

// Get state changes since a specific sequence number
recentChanges := entanglement.GetStateHistory(5)
```

## Use Cases

### Distributed Data Processing

Perfect for scenarios where multiple jobs process parts of a larger dataset and need to share intermediate results:

```go
pool.Schedule("partition-1", func() (any, error) {
    data := processPartition1()
    entanglement.UpdateState("part1-result", data)
    return data, nil
})

pool.Schedule("partition-2", func() (any, error) {
    data := processPartition2()
    entanglement.UpdateState("part2-result", data)
    return data, nil
})

pool.Schedule("aggregate", func() (any, error) {
    part1, _ := entanglement.GetState("part1-result")
    part2, _ := entanglement.GetState("part2-result")
    return combineResults(part1, part2), nil
})
```

### Coordinated Task Execution

Useful for tasks that need to coordinate their actions while maintaining independence:

```go
pool.Schedule("monitor", func() (any, error) {
    for {
        metrics := collectMetrics()
        entanglement.UpdateState("system-metrics", metrics)
        time.Sleep(time.Second)
    }
})

pool.Schedule("scaler", func() (any, error) {
    if metrics, exists := entanglement.GetState("system-metrics"); exists {
        adjustScale(metrics)
    }
    return nil, nil
})
```

## Best Practices

1. **Use Meaningful State Keys**: Choose descriptive keys that clearly indicate the type and purpose of the stored data.

2. **Handle State Absence**: Always check if state exists using the second return value from `GetState`.

3. **Clean Up**: Use appropriate TTL values to ensure entanglements are cleaned up when no longer needed.

4. **State Size**: Keep shared state reasonably sized as it persists in memory.

5. **Error Handling**: Include error states in your shared state to help coordinate error handling across jobs.

## Thread Safety

All Entanglement operations are thread-safe. The implementation uses mutex locks to ensure safe concurrent access to shared state and the state ledger.

## Performance Considerations

- State changes are stored in memory
- Each state change is recorded in the ledger
- Consider the frequency of state updates in your design
- Use appropriate TTL values to manage memory usage

## Limitations

- State is not persisted to disk
- All state is kept in memory
- State size should be reasonable
- Not suitable for sharing large data sets
