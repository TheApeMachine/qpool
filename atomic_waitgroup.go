package qpool

import (
	"runtime"
	"sync/atomic"
)

type atomicWaitGroup struct {
	count atomic.Int64
}

func (wg *atomicWaitGroup) Add(delta int64) {
	wg.count.Add(delta)
}

func (wg *atomicWaitGroup) Done() {
	wg.Add(-1)
}

func (wg *atomicWaitGroup) Wait() {
	for wg.count.Load() > 0 {
		runtime.Gosched()
	}
}
