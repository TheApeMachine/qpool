package qpool

import (
	"sync/atomic"
)

/*
LogController controls qpool's standard logger emission.
*/
type LogController struct {
	suppressed atomic.Int64
}

var defaultLogController = &LogController{}

/*
SuppressLogging disables qpool and errnie standard logging until the returned
restore function is called.
*/
func SuppressLogging() func() {
	return defaultLogController.Suppress()
}

/*
Suppress disables standard logging for this controller.
*/
func (controller *LogController) Suppress() func() {
	if controller == nil {
		return func() {}
	}

	controller.suppressed.Add(1)

	return func() {
		for {
			current := controller.suppressed.Load()

			if current <= 0 {
				return
			}

			if controller.suppressed.CompareAndSwap(current, current-1) {
				return
			}
		}
	}
}

/*
Suppressed reports whether standard logging is currently disabled.
*/
func (controller *LogController) Suppressed() bool {
	if controller == nil {
		return false
	}

	return controller.suppressed.Load() > 0
}
