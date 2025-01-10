package qpool

import "math"

type Qubit struct {
	alpha           complex128 // |0⟩ amplitude
	beta            complex128 // |1⟩ amplitude
	decoherenceRate float64
}

func NewQubit(alpha, beta complex128) *Qubit {
	return &Qubit{
		alpha:           alpha,
		beta:            beta,
		decoherenceRate: 0.01,
	}
}

func (q *Qubit) ApplyHadamard() {
	// H = 1/√2 * [1  1]
	//           [1 -1]
	newAlpha := (q.alpha + q.beta) / complex(math.Sqrt(2), 0)
	newBeta := (q.alpha - q.beta) / complex(math.Sqrt(2), 0)
	q.alpha = newAlpha
	q.beta = newBeta
}
