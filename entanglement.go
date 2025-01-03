package qpool

import (
	"sync"
	"time"
)

/*
Entanglement wraps a selection of jobs into a shared space.
Meant for jobs that each describe part of a larger task.
When one job in the entanglement changes state, it affects all others.

Inspired by quantum entanglement, this type provides a way to create groups of jobs
that share state and react to changes in that state simultaneously. Just as quantum
particles can be entangled such that the state of one instantly affects the other,
jobs in an Entanglement share a common state that, when changed, affects all jobs
in the group.

The shared state in an Entanglement is immutable and persistent. State changes are
recorded in a ledger and replayed for any job that joins or starts processing later,
ensuring that the quantum-like property of entanglement is maintained across time.
This means that even if jobs process at different times, they all see the complete
history of state changes, preserving the causal relationship between entangled jobs.

Use cases include:
  - Distributed data processing where multiple jobs need to share intermediate results
  - Coordinated tasks where jobs need to react to each other's progress
  - State synchronization across a set of related operations
  - Fan-out/fan-in patterns where multiple jobs contribute to a shared outcome
*/
type Entanglement struct {
	ID            string
	Jobs          []Job
	SharedState   map[string]any
	CreatedAt     time.Time
	LastModified  time.Time
	mu            sync.RWMutex
	Dependencies  []string
	TTL           time.Duration
	OnStateChange func(oldState, newState map[string]any)
	
	// StateChangeLedger maintains an ordered history of all state changes
	// This ensures that even jobs that start processing later will see
	// the complete history of state changes in the correct order
	stateLedger []StateChange
}

/*
StateChange represents an immutable record of a change to the shared state.
Each change is timestamped and contains both the key and value that was changed,
allowing for precise replay of state evolution.
*/
type StateChange struct {
	Timestamp time.Time
	Key       string
	Value     any
	Sequence  uint64 // Monotonically increasing sequence number
}

/*
NewEntanglement creates a new entanglement of jobs with the specified ID and TTL.

The entanglement acts as a quantum-inspired container that maintains shared state
across multiple jobs. Like quantum entangled particles that remain connected regardless
of distance, jobs in the entanglement remain connected through their shared state.

The state history is preserved and replayed for any job that starts processing later,
ensuring that the quantum-like property of entanglement is maintained across time.

Parameters:
  - id: A unique identifier for the entanglement
  - jobs: Initial set of jobs to be entangled
  - ttl: Time-to-live duration after which the entanglement expires

Example:
    jobs := []Job{job1, job2, job3}
    entanglement := NewEntanglement("data-processing", jobs, 1*time.Hour)
*/
func NewEntanglement(id string, jobs []Job, ttl time.Duration) *Entanglement {
	return &Entanglement{
		ID:           id,
		Jobs:         jobs,
		SharedState:  make(map[string]any),
		CreatedAt:    time.Now(),
		LastModified: time.Now(),
		TTL:          ttl,
		stateLedger:  make([]StateChange, 0),
	}
}

/*
UpdateState updates the shared state and notifies all entangled jobs of the change.

Similar to how measuring one quantum particle instantly affects its entangled partner,
updating state through this method instantly affects all jobs in the entanglement.
The state change is recorded in an immutable ledger, ensuring that even jobs that
haven't started processing yet will see this change when they begin.

The OnStateChange callback (if set) is triggered with both the old and new state,
allowing jobs to react to the change. For jobs that start later, these changes
are replayed in order during their initialization.

Parameters:
  - key: The state key to update
  - value: The new value for the state key

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (e *Entanglement) UpdateState(key string, value any) {
	e.mu.Lock()
	defer e.mu.Unlock()

	oldState := make(map[string]any)
	for k, v := range e.SharedState {
		oldState[k] = v
	}

	// Record the state change in the ledger
	change := StateChange{
		Timestamp: time.Now(),
		Key:       key,
		Value:     value,
		Sequence:  uint64(len(e.stateLedger)),
	}
	e.stateLedger = append(e.stateLedger, change)

	// Update the current state
	e.SharedState[key] = value
	e.LastModified = change.Timestamp

	if e.OnStateChange != nil {
		e.OnStateChange(oldState, e.SharedState)
	}
}

/*
GetStateHistory returns all state changes that have occurred since a given sequence number.
This allows jobs that start processing later to catch up on all state changes they missed.

Parameters:
  - sinceSequence: The sequence number to start from (0 for all history)

Returns:
  - []StateChange: Ordered list of state changes since the specified sequence
*/
func (e *Entanglement) GetStateHistory(sinceSequence uint64) []StateChange {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if sinceSequence >= uint64(len(e.stateLedger)) {
		return []StateChange{}
	}

	return e.stateLedger[sinceSequence:]
}

/*
ReplayStateChanges applies all historical state changes to a newly starting job.
This ensures that jobs starting later still see the complete history of state
changes in the correct order.

Parameters:
  - job: The job to replay state changes for
*/
func (e *Entanglement) ReplayStateChanges(job Job) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	history := e.GetStateHistory(0)
	currentState := make(map[string]any)

	for _, change := range history {
		oldState := make(map[string]any)
		for k, v := range currentState {
			oldState[k] = v
		}

		// Create a new map for the new state
		newState := make(map[string]any)
		for k, v := range currentState {
			newState[k] = v
		}
		newState[change.Key] = change.Value

		currentState = newState
		if e.OnStateChange != nil {
			e.OnStateChange(oldState, newState)
		}
	}
}

/*
GetState retrieves a value from the shared state.

This method provides a way to observe the current state of the entanglement.
Like quantum measurement, it provides a snapshot of the current state at the
time of observation.

Parameters:
  - key: The state key to retrieve

Returns:
  - value: The value associated with the key
  - exists: Boolean indicating whether the key exists in the state

Thread-safe: This method uses a read lock to ensure safe concurrent access.
*/
func (e *Entanglement) GetState(key string) (any, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	value, exists := e.SharedState[key]
	return value, exists
}

/*
AddJob adds a job to the entanglement.

This method expands the entanglement to include a new job, similar to how
quantum systems can be expanded to include more entangled particles. The newly
added job becomes part of the shared state system and will be affected by
state changes.

Parameters:
  - job: The job to add to the entanglement

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (e *Entanglement) AddJob(job Job) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.Jobs = append(e.Jobs, job)
	e.LastModified = time.Now()
}

/*
RemoveJob removes a job from the entanglement.

This method removes a job from the entanglement, effectively "disentangling" it
from the shared state system. The removed job will no longer be affected by or
contribute to state changes in the entanglement.

Parameters:
  - jobID: The ID of the job to remove

Returns:
  - bool: True if the job was found and removed, false otherwise

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (e *Entanglement) RemoveJob(jobID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i, job := range e.Jobs {
		if job.ID == jobID {
			e.Jobs = append(e.Jobs[:i], e.Jobs[i+1:]...)
			e.LastModified = time.Now()
			return true
		}
	}
	return false
}

/*
IsExpired checks if the entanglement has exceeded its TTL.

This method determines if the entanglement should be considered expired based on
its Time-To-Live (TTL) duration and the time since its last modification. An
expired entanglement might be cleaned up by the system, similar to how quantum
entanglement can be lost due to decoherence.

Returns:
  - bool: True if the entanglement has expired, false otherwise

Note: A TTL of 0 or less means the entanglement never expires.
*/
func (e *Entanglement) IsExpired() bool {
	if e.TTL <= 0 {
		return false
	}
	return time.Since(e.LastModified) > e.TTL
}

