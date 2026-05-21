package qpool

import (
	"time"

	"github.com/theapemachine/errnie"
)

/*
QValue is a stored job result or broadcast payload with optional error metadata.
*/
type QValue[T any] struct {
	SenderID   string
	ReceiverID string
	Value      T
	Error      error
	CreatedAt  int64
	TTL        time.Duration
}

/*
NewQValue constructs a value with a creation timestamp.
*/
func NewQValue[T any](
	senderID string,
	receiverID string,
	initialValue T,
	ttl time.Duration,
) (*QValue[T], error) {
	value := &QValue[T]{
		SenderID:   senderID,
		ReceiverID: receiverID,
		Value:      initialValue,
		CreatedAt:  time.Now().UnixNano(),
		TTL:        ttl,
	}

	// Value may be nil for error-only results; do not pass it to Require.
	return value, errnie.Require(
		map[string]any{
			"SenderID":   value.SenderID,
			"ReceiverID": value.ReceiverID,
			"CreatedAt":  value.CreatedAt,
			"TTL":        value.TTL,
		},
	)
}
