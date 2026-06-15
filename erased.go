package qpool

import "unsafe"

// erasedAny names the empty interface without shadowing the predeclared any
// identifier inside methods on Q[erasedAny].
type erasedAny = interface{}

// qAny reinterprets *Q[T] as the type-erased pool handle used by disruptor
// workers, the scaler, and other internal paths. Q's memory layout does not
// depend on T.
func qAny[T any](q *Q[T]) *Q[erasedAny] {
	return (*Q[erasedAny])(unsafe.Pointer(q))
}
