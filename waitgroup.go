package qpool

import (
	"sync/atomic"
)

type WaitGroup struct {
	count atomic.Int64
	sema  uint32
}

func (wg *WaitGroup) Add(delta int64) {
	next := wg.count.Add(delta)
	if next == 0 {
		runtime_Semrelease(&wg.sema, false, 0)
	}
}

func (wg *WaitGroup) Done() {
	wg.Add(-1)
}

func (wg *WaitGroup) Wait() {
	for wg.count.Load() > 0 {
		runtime_Semacquire(&wg.sema)
	}
}
