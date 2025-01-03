// qspace.go
package qpool

import (
	"fmt"
	"math"
	"sync"
	"time"
)

/*
	StateTransition represents a change in quantum state.

It records the complete history of state changes, including the previous and new states,
timing information, and the cause of the transition. This maintains a quantum-like
state history that can be used to understand the evolution of values over time.
*/
type StateTransition struct {
	ValueID   string
	FromState State
	ToState   State
	Timestamp time.Time
	Cause     string
}

/*
	UncertaintyPrinciple enforces quantum-like uncertainty rules.

It implements Heisenberg-inspired uncertainty principles where observation and
time affect the certainty of quantum values. The principle ensures that values
become more uncertain over time and that frequent observations impact the
system's state.

Key concepts:
  - Minimum time between observations to limit measurement impact
  - Maximum time before reaching maximum uncertainty
  - Base uncertainty level for all measurements
*/
type UncertaintyPrinciple struct {
	MinDeltaTime    time.Duration // Minimum time between observations
	MaxDeltaTime    time.Duration // Maximum time before max uncertainty
	BaseUncertainty UncertaintyLevel
}

/*
	QSpace represents a quantum-like state space.

It provides a managed environment for quantum-inspired values, maintaining their
states, relationships, and uncertainties. The space implements concepts from
quantum mechanics such as entanglement, state superposition, and the uncertainty
principle.

Key features:
  - Quantum value storage and retrieval
  - State transition history
  - Entanglement management
  - Relationship tracking
  - Automatic resource cleanup
*/
type QSpace struct {
	mu sync.RWMutex

	// Core storage
	values  map[string]*QValue
	waiting map[string][]chan *QValue
	groups  map[string]*BroadcastGroup

	// Quantum properties
	entanglements map[string]*Entanglement
	stateHistory  []StateTransition
	uncertainty   *UncertaintyPrinciple

	// Relationship tracking
	children map[string][]string
	parents  map[string][]string

	// Cleanup and maintenance
	cleanupInterval time.Duration
	wg              sync.WaitGroup
	done            chan struct{}
}

/*
	NewQSpace creates a new quantum space.

Initializes a new space with default uncertainty principles and starts
maintenance goroutines for cleanup and uncertainty monitoring.

Returns:
  - *QSpace: A new quantum space instance ready for use
*/
func NewQSpace() *QSpace {
	qs := &QSpace{
		values:        make(map[string]*QValue),
		waiting:       make(map[string][]chan *QValue),
		groups:        make(map[string]*BroadcastGroup),
		entanglements: make(map[string]*Entanglement),
		children:      make(map[string][]string),
		parents:       make(map[string][]string),
		uncertainty: &UncertaintyPrinciple{
			MinDeltaTime:    time.Millisecond * 100,
			MaxDeltaTime:    time.Second * 10,
			BaseUncertainty: UncertaintyLevel(0.1),
		},
		cleanupInterval: time.Minute,
		done:            make(chan struct{}),
	}

	// Start maintenance goroutines
	qs.wg.Add(2)
	go qs.runCleanup()
	go qs.monitorUncertainty()

	return qs
}

/*
	Store stores a quantum value with proper uncertainty handling.

Values are stored with their associated states and TTL, maintaining
quantum-inspired properties such as superposition and uncertainty.

Parameters:
  - id: Unique identifier for the value
  - value: The actual value to store
  - states: Possible states with their probabilities
  - ttl: Time-to-live duration for the value

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (qs *QSpace) Store(id string, value interface{}, states []State, ttl time.Duration) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	// Create new quantum value
	qv := NewQValue(value, states)
	qv.TTL = ttl

	// Record state transition
	if oldQV, exists := qs.values[id]; exists {
		qs.recordTransition(id, oldQV.States[0], states[0], "store")
	}

	qs.values[id] = qv

	// Handle entanglements
	if entangled := qs.entanglements[id]; entangled != nil {
		entangled.UpdateState("value", qv.Value)
	}

	// Notify waiting observers
	if channels, ok := qs.waiting[id]; ok {
		for _, ch := range channels {
			select {
			case ch <- qv:
				// Value successfully sent
			default:
				// Channel full or closed, remove it
				qs.removeWaitingChannel(id, ch)
			}
		}
		delete(qs.waiting, id)
	}
}

/*
	Await returns a channel that will receive the quantum value.

Implements quantum-inspired delayed observation, where values may be uncertain
or not yet collapsed to a definite state.

Parameters:
  - id: The identifier of the value to await

Returns:
  - chan *QValue: Channel that will receive the value when available

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (qs *QSpace) Await(id string) chan *QValue {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	ch := make(chan *QValue, 1)

	// Check if value exists
	if qv, ok := qs.values[id]; ok {
		// Value exists but might be uncertain
		uncertainty := qv.Uncertainty
		if uncertainty < MaxUncertainty {
			ch <- qv
			close(ch)
			return ch
		}
	}

	// Add to waiting list
	qs.waiting[id] = append(qs.waiting[id], ch)
	return ch
}

/*
	CreateEntanglement establishes quantum entanglement between values.

Creates and manages relationships between quantum values that should maintain
synchronized states. Like quantum entanglement in physics, changes to one value
affect all entangled values.

Parameters:
  - ids: Slice of value IDs to entangle together

Returns:
  - *Entanglement: A new entanglement instance managing the relationship

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (qs *QSpace) CreateEntanglement(ids []string) *Entanglement {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	// Create jobs from IDs
	jobs := make([]Job, len(ids))
	for i, id := range ids {
		jobs[i] = Job{ID: id}
	}

	// Create new entanglement with default 1 hour TTL
	ent := NewEntanglement(ids[0], jobs, time.Hour)

	// Link values to entanglement
	for _, id := range ids {
		if qv, exists := qs.values[id]; exists {
			ent.UpdateState(id, qv.Value)
		}
		qs.entanglements[id] = ent
	}

	return ent
}

/*
	recordTransition records a state transition in history.

Maintains an immutable record of state changes for quantum values, allowing
for analysis of state evolution over time.

Parameters:
  - valueID: ID of the value that changed state
  - from: Previous state
  - to: New state
  - cause: Reason for the state transition

Thread-safe: Called within Store which provides mutex protection.
*/
func (qs *QSpace) recordTransition(valueID string, from, to State, cause string) {
	transition := StateTransition{
		ValueID:   valueID,
		FromState: from,
		ToState:   to,
		Timestamp: time.Now(),
		Cause:     cause,
	}
	qs.stateHistory = append(qs.stateHistory, transition)
}

/*
	runCleanup periodically cleans up expired values.

Runs as a background goroutine to maintain the quantum space by removing
expired values and relationships, preventing resource leaks.

Thread-safe: Uses internal mutex protection for cleanup operations.
*/
func (qs *QSpace) runCleanup() {
	defer qs.wg.Done()
	ticker := time.NewTicker(qs.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-qs.done:
			return
		case <-ticker.C:
			qs.cleanup()
		}
	}
}

/*
	monitorUncertainty updates uncertainty levels based on time.

Implements quantum-inspired uncertainty principles by adjusting uncertainty
levels of values based on time elapsed since last observation.

Thread-safe: Uses internal mutex protection for uncertainty updates.
*/
func (qs *QSpace) monitorUncertainty() {
	defer qs.wg.Done()
	ticker := time.NewTicker(qs.uncertainty.MinDeltaTime)
	defer ticker.Stop()

	for {
		select {
		case <-qs.done:
			return
		case <-ticker.C:
			qs.updateUncertainties()
		}
	}
}

/*
	cleanup removes expired values and updates relationships.

Performs the actual cleanup operations, removing expired values and updating
relationship mappings to maintain consistency.

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (qs *QSpace) cleanup() {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	now := time.Now()
	for id, qv := range qs.values {
		if qv.TTL > 0 && now.Sub(qv.CreatedAt) > qv.TTL {
			// Clean up relationships
			delete(qs.values, id)
			delete(qs.entanglements, id)
			delete(qs.children, id)

			// Remove from parent relationships
			for parentID, children := range qs.children {
				qs.children[parentID] = removeString(children, id)
			}
		}
	}

	// Cleanup broadcast groups
	for id, group := range qs.groups {
		if group.TTL > 0 && now.Sub(group.LastUsed) > group.TTL {
			for _, ch := range group.channels {
				close(ch)
			}
			delete(qs.groups, id)
		}
	}
}

/*
	updateUncertainties updates uncertainty for all quantum values.

Implements the time-based uncertainty principle where values become more
uncertain as time passes since their last observation.

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (qs *QSpace) updateUncertainties() {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	for _, qv := range qs.values {
		if !qv.isCollapsed {
			continue
		}

		// Calculate time-based uncertainty increase
		timeSinceCollapse := time.Since(qv.collapseTime)
		if timeSinceCollapse > qs.uncertainty.MaxDeltaTime {
			qv.Uncertainty = MaxUncertainty
			continue
		}

		// Progressive uncertainty increase
		progressFactor := float64(timeSinceCollapse) / float64(qs.uncertainty.MaxDeltaTime)
		uncertaintyIncrease := UncertaintyLevel(progressFactor * float64(qs.uncertainty.BaseUncertainty))
		qv.Uncertainty = UncertaintyLevel(math.Min(
			float64(qv.Uncertainty+uncertaintyIncrease),
			float64(MaxUncertainty),
		))
	}
}

/*
	removeWaitingChannel removes a channel from the waiting list.

Maintains the waiting channel list by safely removing closed or unneeded channels.

Parameters:
  - id: The value ID associated with the channel
  - ch: The channel to remove

Thread-safe: Called within Store which provides mutex protection.
*/
func (qs *QSpace) removeWaitingChannel(id string, ch chan *QValue) {
	channels := qs.waiting[id]
	for i, waitingCh := range channels {
		if waitingCh == ch {
			qs.waiting[id] = append(channels[:i], channels[i+1:]...)
			return
		}
	}
}

/*
	GetStateHistory returns the state transition history for a value.

Provides access to the complete history of state transitions for analysis
and debugging purposes.

Parameters:
  - valueID: ID of the value to get history for

Returns:
  - []StateTransition: Ordered list of state transitions for the value

Thread-safe: This method uses read-lock to ensure safe concurrent access.
*/
func (qs *QSpace) GetStateHistory(valueID string) []StateTransition {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	var history []StateTransition
	for _, transition := range qs.stateHistory {
		if transition.ValueID == valueID {
			history = append(history, transition)
		}
	}
	return history
}

/*
	AddRelationship establishes a parent-child relationship between values.

Creates directed relationships between values while preventing circular
dependencies that could cause deadlocks or infinite loops.

Parameters:
  - parentID: ID of the parent value
  - childID: ID of the child value

Returns:
  - error: Error if the relationship would create a circular dependency

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (qs *QSpace) AddRelationship(parentID, childID string) error {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	// Check for circular dependencies
	if qs.wouldCreateCircle(parentID, childID) {
		return fmt.Errorf("circular dependency detected")
	}

	qs.children[parentID] = append(qs.children[parentID], childID)
	qs.parents[childID] = append(qs.parents[childID], parentID)
	return nil
}

/*
	wouldCreateCircle checks if adding a relationship would create a circular dependency.

Performs depth-first search to detect potential circular dependencies before
they are created.

Parameters:
  - parentID: Proposed parent ID
  - childID: Proposed child ID

Returns:
  - bool: True if adding the relationship would create a circle

Thread-safe: Called within AddRelationship which provides mutex protection.
*/
func (qs *QSpace) wouldCreateCircle(parentID, childID string) bool {
	visited := make(map[string]bool)
	var checkCircular func(string) bool

	checkCircular = func(current string) bool {
		if current == parentID {
			return true
		}
		if visited[current] {
			return false
		}
		visited[current] = true

		for _, parent := range qs.parents[current] {
			if checkCircular(parent) {
				return true
			}
		}
		return false
	}

	return checkCircular(childID)
}

/*
	Close shuts down the quantum space.

Performs graceful shutdown of the quantum space, cleaning up resources
and closing all channels safely.

Thread-safe: This method uses mutual exclusion to ensure safe concurrent access.
*/
func (qs *QSpace) Close() {
	close(qs.done)
	qs.wg.Wait()

	qs.mu.Lock()
	defer qs.mu.Unlock()

	// Close all waiting channels
	for _, channels := range qs.waiting {
		for _, ch := range channels {
			close(ch)
		}
	}

	// Close all broadcast group channels
	for _, group := range qs.groups {
		for _, ch := range group.channels {
			close(ch)
		}
	}

	// Clear maps
	qs.values = nil
	qs.waiting = nil
	qs.groups = nil
	qs.entanglements = nil
	qs.children = nil
	qs.parents = nil
}

/*
	removeString removes a string from a slice.

Helper function to remove a string from a slice.
*/
func removeString(slice []string, s string) []string {
	for i, v := range slice {
		if v == s {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

/*
	CreateBroadcastGroup creates a new broadcast group.

Initializes a new broadcast group with specified parameters and default
quantum properties such as minimum uncertainty.
*/
func (qs *QSpace) CreateBroadcastGroup(id string, ttl time.Duration) *BroadcastGroup {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	group := NewBroadcastGroup(id, ttl, 100) // Default max queue size of 100
	qs.groups[id] = group
	return group
}

/*
	Subscribe returns a channel for receiving values from a broadcast group.

Provides a channel for receiving quantum values from a specific broadcast group.
*/
func (qs *QSpace) Subscribe(groupID string) chan *QValue {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	if group, exists := qs.groups[groupID]; exists {
		return group.Subscribe("", 10) // Default buffer size of 10
	}
	return nil
}

/*
	Exists checks if a value exists in the space
*/
func (qs *QSpace) Exists(id string) bool {
	qs.mu.RLock()
	defer qs.mu.RUnlock()
	_, exists := qs.values[id]
	return exists
}

/*
	StoreError stores an error result in the quantum space
*/
func (qs *QSpace) StoreError(id string, err error, ttl time.Duration) {
	qs.mu.Lock()
	defer qs.mu.Unlock()

	// Create new quantum value with error
	qv := NewQValue(nil, []State{{Value: nil, Probability: 1.0}})
	qv.Error = err
	qv.TTL = ttl

	// Record state transition if value existed
	if oldQV, exists := qs.values[id]; exists {
		qs.recordTransition(id, oldQV.States[0], qv.States[0], "error")
	}

	qs.values[id] = qv

	// Notify waiting observers
	if channels, ok := qs.waiting[id]; ok {
		for _, ch := range channels {
			select {
			case ch <- qv:
				// Value successfully sent
			default:
				// Channel full or closed, remove it
				qs.removeWaitingChannel(id, ch)
			}
		}
		delete(qs.waiting, id)
	}
}
