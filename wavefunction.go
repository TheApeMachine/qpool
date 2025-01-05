// wavefunction.go
package qpool

import (
	"math"
	"math/rand"

	"github.com/theapemachine/errnie"
)

/*
WaveFunction represents a quantum state that can exist in multiple possible
states simultaneously until observation or verification forces a collapse
into a definite state.
*/
type WaveFunction struct {
	States      []State
	Uncertainty UncertaintyLevel
	isCollapsed bool

	// New fields for verification-aware collapse
	methodDiversity float64
	evidenceQuality float64
}

/*
Evidence represents verification data supporting a particular state.
*/
type Evidence struct {
	Method     string      // Verification method used
	Confidence float64     // Confidence in this evidence
	Data       interface{} // The actual evidence data
}

func NewWaveFunction(
	states []State,
	uncertainty UncertaintyLevel,
	methodDiversity float64,
) *WaveFunction {
	errnie.Info(
		"NewWaveFunction - states %v, uncertainty %v, methodDiversity %v",
		states,
		uncertainty,
		methodDiversity,
	)
	return &WaveFunction{
		States:          states,
		Uncertainty:     uncertainty,
		isCollapsed:     false,
		methodDiversity: methodDiversity,
	}
}

/*
Collapse forces the wave function to choose a definite state based on both
probabilities and verification evidence. The collapse mechanism considers:
1. State probabilities
2. Method diversity
3. Evidence quality
4. Uncertainty level
*/
func (wf *WaveFunction) Collapse() interface{} {
	if wf.isCollapsed {
		if len(wf.States) > 0 {
			return wf.States[0].Value
		}
		return nil
	}

	if len(wf.States) == 0 {
		return nil
	}

	// Calculate adjusted probabilities based on evidence
	adjustedStates := wf.calculateAdjustedProbabilities()

	// Generate a random number between 0 and 1
	r := rand.Float64()

	// Collapse based on adjusted probabilities
	var cumulativeProb float64
	for _, state := range adjustedStates {
		cumulativeProb += state.Probability
		if r <= cumulativeProb {
			// Collapse to this state
			wf.States = []State{state}
			wf.isCollapsed = true

			// Uncertainty reduces after collapse
			wf.Uncertainty = UncertaintyLevel(
				math.Max(0.1, float64(wf.Uncertainty)*(1.0-wf.methodDiversity)),
			)

			return state.Value
		}
	}

	// Fallback collapse
	lastState := adjustedStates[len(adjustedStates)-1]
	wf.States = []State{lastState}
	wf.isCollapsed = true
	return lastState.Value
}

/*
calculateAdjustedProbabilities modifies state probabilities based on
verification evidence and method diversity.
*/
func (wf *WaveFunction) calculateAdjustedProbabilities() []State {
	adjustedStates := make([]State, len(wf.States))
	copy(adjustedStates, wf.States)

	// Calculate evidence-based adjustments
	for i := range adjustedStates {
		evidenceWeight := wf.calculateEvidenceWeight(adjustedStates[i].Evidence)

		// Adjust probability based on evidence and method diversity
		adjustedStates[i].Probability *= (1 + evidenceWeight*wf.methodDiversity)
	}

	// Normalize probabilities
	wf.normalizeStateProbabilities(adjustedStates)

	return adjustedStates
}

/*
calculateEvidenceWeight determines how much evidence should influence
the collapse probability.
*/
func (wf *WaveFunction) calculateEvidenceWeight(evidence []Evidence) float64 {
	if len(evidence) == 0 {
		return 0
	}

	var totalWeight float64
	for _, e := range evidence {
		totalWeight += e.Confidence
	}

	return totalWeight / float64(len(evidence))
}

/*
normalizeStateProbabilities ensures probabilities sum to 1.0
*/
func (wf *WaveFunction) normalizeStateProbabilities(states []State) {
	var total float64
	for _, s := range states {
		total += s.Probability
	}

	if total > 0 {
		for i := range states {
			states[i].Probability /= total
		}
	}
}

/*
AddEvidence allows adding new evidence to a state after creation.
*/
func (wf *WaveFunction) AddEvidence(stateValue interface{}, evidence Evidence) {
	for i, state := range wf.States {
		if state.Value == stateValue {
			wf.States[i].Evidence = append(wf.States[i].Evidence, evidence)
			return
		}
	}
}

/*
UpdateMethodDiversity allows updating the method diversity score as new
verification methods are added.
*/
func (wf *WaveFunction) UpdateMethodDiversity(diversity float64) {
	wf.methodDiversity = diversity
	// Higher diversity reduces uncertainty
	wf.Uncertainty = UncertaintyLevel(
		math.Max(0.1, float64(wf.Uncertainty)*(1.0-diversity)),
	)
}
