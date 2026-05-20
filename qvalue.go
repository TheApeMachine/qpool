package qpool

import (
	"fmt"
	"time"
)

/*
QValue is a stored job result or broadcast payload with optional error metadata.
*/
type QValue struct {
	Value     interface{}
	Error     error
	CreatedAt time.Time
	TTL       time.Duration
}

/*
NewQValue constructs a value with a creation timestamp.
*/
func NewQValue(initialValue interface{}) *QValue {
	return &QValue{
		Value:     initialValue,
		CreatedAt: time.Now(),
	}
}

/*
ID returns a diagnostic string (not guaranteed globally unique).
*/
func (qv *QValue) ID() string {
	return fmt.Sprintf("qv_%v_%d", qv.Value, qv.CreatedAt.UnixNano())
}
