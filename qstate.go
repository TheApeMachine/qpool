package qpool

import (
	"math/cmplx"
	"math/rand"
	"time"
)

type QuantumState struct {
	Vector      []complex128
	Uncertainty float64
}

func (qs *QuantumState) Collapse() any {
	// Generic state collapse that returns interface{}
	return qs.Measure()
}

func (qs *QuantumState) Measure() any {
	n := len(qs.Vector)
	if n == 0 {
		return nil // Handle empty quantum state
	}

	// Calculate the probabilities for each state
	probs := make([]float64, n)
	totalProb := 0.0
	for i, amplitude := range qs.Vector {
		prob := cmplx.Abs(amplitude)
		prob *= prob // Square of the modulus
		probs[i] = prob
		totalProb += prob
	}

	// Normalize the probabilities
	for i := range probs {
		probs[i] /= totalProb
	}

	// Generate a random number to simulate measurement
	rand.Seed(time.Now().UnixNano())
	r := rand.Float64()

	// Determine the measured state based on the probabilities
	cumulativeProb := 0.0
	measuredState := 0
	for i, prob := range probs {
		cumulativeProb += prob
		if r <= cumulativeProb {
			measuredState = i
			break
		}
	}

	// Collapse the quantum state to the measured state
	collapsedVector := make([]complex128, n)
	collapsedVector[measuredState] = 1 + 0i // Set the measured state to 1
	qs.Vector = collapsedVector

	// Return the index of the measured state
	return measuredState
}
