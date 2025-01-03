// qvalue.go
package qpool

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

// Probability represents a quantum state probability
type Probability float64

// UncertaintyLevel defines how uncertain we are about a value
type UncertaintyLevel float64

const (
	MinUncertainty UncertaintyLevel = 0.0
	MaxUncertainty UncertaintyLevel = 1.0
)

// ObservationEffect represents how observation affects the quantum state
type ObservationEffect struct {
	ObserverID    string
	ObservedAt    time.Time
	StateCollapse bool
	Uncertainty   UncertaintyLevel
}

// QValue represents a value with quantum-like properties
type QValue struct {
	mu sync.RWMutex

	// Core value and metadata
	Value     interface{}
	Error     error
	CreatedAt time.Time
	TTL       time.Duration

	// Quantum properties
	States       []State          // Possible superposition states
	Uncertainty  UncertaintyLevel // Heisenberg-inspired uncertainty
	Observations []ObservationEffect
	Entangled    []string // IDs of entangled values

	// Wave function collapse tracking
	isCollapsed  bool
	collapseTime time.Time
}

// NewQValue creates a new quantum value with initial states
func NewQValue(initialValue interface{}, states []State) *QValue {
	qv := &QValue{
		Value:       initialValue,
		CreatedAt:   time.Now(),
		States:      states,
		Uncertainty: calculateInitialUncertainty(states),
		isCollapsed: false,
	}
	return qv
}

// Observe triggers wave function collapse based on quantum rules
func (qv *QValue) Observe(observerID string) interface{} {
	qv.mu.Lock()
	defer qv.mu.Unlock()

	observation := ObservationEffect{
		ObserverID:    observerID,
		ObservedAt:    time.Now(),
		Uncertainty:   qv.Uncertainty,
		StateCollapse: !qv.isCollapsed,
	}
	qv.Observations = append(qv.Observations, observation)

	// First observation collapses the wave function
	if !qv.isCollapsed {
		qv.collapse()
	}

	// Increase uncertainty based on Heisenberg principle
	qv.updateUncertainty()

	return qv.Value
}

// collapse performs wave function collapse, choosing a state based on probabilities
func (qv *QValue) collapse() {
	if len(qv.States) == 0 {
		return
	}

	// Calculate cumulative probabilities
	cumProb := 0.0
	probs := make([]float64, len(qv.States))
	for i, state := range qv.States {
		cumProb += float64(state.Probability)
		probs[i] = cumProb
	}

	// Normalize probabilities
	for i := range probs {
		probs[i] /= cumProb
	}

	// Random selection based on probabilities
	r := rand.Float64()
	for i, threshold := range probs {
		if r <= threshold {
			qv.Value = qv.States[i].Value
			break
		}
	}

	qv.isCollapsed = true
	qv.collapseTime = time.Now()
}

// updateUncertainty increases uncertainty based on time since collapse
func (qv *QValue) updateUncertainty() {
	if !qv.isCollapsed {
		qv.Uncertainty = MaxUncertainty
		return
	}

	timeSinceCollapse := time.Since(qv.collapseTime)
	uncertaintyIncrease := UncertaintyLevel(math.Log1p(float64(timeSinceCollapse.Nanoseconds())))
	qv.Uncertainty = UncertaintyLevel(math.Min(
		float64(qv.Uncertainty+uncertaintyIncrease),
		float64(MaxUncertainty),
	))
}

// calculateInitialUncertainty determines starting uncertainty based on state count
func calculateInitialUncertainty(states []State) UncertaintyLevel {
	if len(states) <= 1 {
		return MinUncertainty
	}
	// More states = more initial uncertainty
	return UncertaintyLevel(math.Log2(float64(len(states))) / 10.0)
}

// Entangle connects this value with another quantum value
func (qv *QValue) Entangle(other *QValue) {
	qv.mu.Lock()
	other.mu.Lock()
	defer qv.mu.Unlock()
	defer other.mu.Unlock()

	// Add bidirectional entanglement
	qv.Entangled = append(qv.Entangled, other.ID())
	other.Entangled = append(other.Entangled, qv.ID())

	// Share states between entangled values
	qv.States = mergeSates(qv.States, other.States)
	other.States = qv.States

	// Increase uncertainty due to entanglement
	entanglementUncertainty := UncertaintyLevel(0.1)
	qv.Uncertainty += entanglementUncertainty
	other.Uncertainty += entanglementUncertainty
}

// mergeSates combines states from two quantum values
func mergeSates(a, b []State) []State {
	seen := make(map[interface{}]bool)
	merged := make([]State, 0)

	// Helper to add unique states
	addState := func(s State) {
		if !seen[s.Value] {
			seen[s.Value] = true
			merged = append(merged, s)
		}
	}

	// Add all states, avoiding duplicates
	for _, s := range a {
		addState(s)
	}
	for _, s := range b {
		addState(s)
	}

	// Normalize probabilities
	totalProb := Probability(0)
	for _, s := range merged {
		totalProb += Probability(s.Probability)
	}
	for i := range merged {
		merged[i].Probability /= float64(totalProb)
	}

	return merged
}

// ID generates a unique identifier for this quantum value
func (qv *QValue) ID() string {
	return fmt.Sprintf("qv_%v_%d", qv.Value, qv.CreatedAt.UnixNano())
}
