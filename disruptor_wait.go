package qpool

import (
	"runtime"
	"time"
)

/*
DisruptorIdleWaitStrategy backs off disruptor listeners when the job queue has
no in-flight work. go-disruptor's default Idle sleeps 500ns per iteration, which
keeps every listener goroutine hot-spinning when the queue is empty.
*/
type DisruptorIdleWaitStrategy struct{}

/*
Gate yields briefly while waiting for an upstream handler group to finish.
*/
func (strategy *DisruptorIdleWaitStrategy) Gate(spinCount int64) {
	runtime.Gosched()
}

/*
Idle escalates from yield to microsleep to millisecond sleep as idlingCount
grows. idlingCount resets to zero as soon as a batch is handled, so the hot
path when jobs are flowing never reaches the longer sleeps.
*/
func (strategy *DisruptorIdleWaitStrategy) Idle(idlingCount int64) {
	if idlingCount <= 64 {
		runtime.Gosched()

		return
	}

	if idlingCount <= 512 {
		time.Sleep(time.Microsecond * time.Duration(idlingCount-64))

		return
	}

	time.Sleep(time.Millisecond)
}

/*
Reserve yields while a producer waits for ring capacity.
*/
func (strategy *DisruptorIdleWaitStrategy) Reserve(spinCount int64) {
	time.Sleep(time.Microsecond)
}
