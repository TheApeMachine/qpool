package qpool

import (
	"sync"
)

/*
LogController controls qpool's standard logger emission.
*/
type LogController struct {
	lock       sync.Mutex
	suppressed int
}

var defaultLogController = &LogController{}

/*
SuppressLogging disables qpool and errnie standard logging until the returned
restore function is called.
*/
func SuppressLogging() func() {
	restoreQPool := defaultLogController.Suppress()

	var once sync.Once

	return func() {
		once.Do(func() {
			restoreQPool()
		})
	}
}

/*
Suppress disables standard logging for this controller.
*/
func (controller *LogController) Suppress() func() {
	if controller == nil {
		return func() {}
	}

	controller.lock.Lock()
	controller.suppressed++
	controller.lock.Unlock()

	var once sync.Once

	return func() {
		once.Do(func() {
			controller.lock.Lock()
			defer controller.lock.Unlock()

			if controller.suppressed == 0 {
				return
			}

			controller.suppressed--
		})
	}
}

/*
Suppressed reports whether standard logging is currently disabled.
*/
func (controller *LogController) Suppressed() bool {
	if controller == nil {
		return false
	}

	controller.lock.Lock()
	defer controller.lock.Unlock()

	return controller.suppressed > 0
}
