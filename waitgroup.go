package qpool

import (
	"runtime"
	"sync/atomic"
)

type WaitGroup struct {
	count atomic.Int64
}

func (wg *WaitGroup) Add(delta int64) {
	wg.count.Add(delta)
}

func (wg *WaitGroup) Done() {
	wg.Add(-1)
}

func (wg *WaitGroup) Wait() {
	for wg.count.Load() > 0 {
		runtime.Gosched()
	}
}
