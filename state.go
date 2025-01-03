package qpool

/*
State represents a possible quantum state with its probability amplitude
and associated verification information.
*/
type State struct {
	Value       interface{}
	Probability float64
	Amplitude   complex128 // For future quantum computing features
	Evidence    []Evidence // Supporting evidence for this state
}
