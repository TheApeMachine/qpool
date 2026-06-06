package qpool

import (
	"sync"
	"sync/atomic"
)

/*
ScheduleFast dispatches an independent job through the low-overhead runtime
executor. It returns the result directly on a lock-free handle and does not
store results in QSpace, publish telemetry, apply retries, dependencies,
regulators, circuit breakers, TTLs, or scaler accounting.
*/
// Invoke invokes the pre-defined method in PoolWithFunc by assigning the data to an already existing worker
// or spawning a new worker given queue size is in limits
func (self *Q[T]) Invoke(value T) {
	var s *slotFunc[T]
	for {
		if s = self.fnPop(); s != nil {
			s.data = value
			safe_ready(s.threadPtr)
			return
		} else if atomic.AddUint64(&self.workerCount, 1) <= uint64(self.maxWorkers) {
			s = &slotFunc[T]{data: value}
			go self.fnLoopQ(s)
			return
		} else {
			atomic.AddUint64(&self.workerCount, uint64SubtractionConstant)
			mcall(gosched_m)
		}
	}
}

// represents the infinite loop for a worker goroutine
func (self *Q[T]) fnLoopQ(d *slotFunc[T]) {
	d.threadPtr = GetG()
	for {
		self.task(d.data)
		self.fnPush(d)
		mcall(fast_park)
	}
}

// pop pops value from the top of the stack
func (self *Q[T]) fnPop() (value *slotFunc[T]) {
	var top, next *dataItem[T]
	for {
		top = self.fnTop.Load()
		if top == nil {
			return
		}
		next = top.next.Load()
		if self.fnTop.CompareAndSwap(top, next) {
			value = top.value
			top.value = nil
			top.next.Store(nil)
			self.free(top)
			return
		}
	}
}

// push pushes a value on top of the stack
func (self *Q[T]) fnPush(v *slotFunc[T]) {
	var (
		top  *dataItem[T]
		item = self.alloc().(*dataItem[T])
	)

	item.value = v

	for {
		top = self.fnTop.Load()
		item.next.Store(top)

		if self.fnTop.CompareAndSwap(top, item) {
			return
		}
	}
}

// Submit submits a new task to the pool
// it first tries to use already parked goroutines from the stack if any
// if there are no available worker goroutines, it tries to add a
// new goroutine to the pool if the pool capacity is not exceeded
// in case the pool capacity hit its maximum limit, this function yields the processor to other
// goroutines and loops again for finding available workers
func (self *Q[T]) ScheduleFast(task func()) {
	var s *slot
	for {
		if s = self.pop(); s != nil {
			s.task = task
			safe_ready(s.threadPtr)
			return
		} else if atomic.AddUint64(&self.workerCount, 1) <= uint64(self.maxWorkers) {
			s = &slot{task: task}
			go self.loopQ(s)
			return
		} else {
			atomic.AddUint64(&self.workerCount, uint64SubtractionConstant)
			mcall(gosched_m)
		}
	}
}

// loopQ is the looping function for every worker goroutine
func (self *Q[T]) loopQ(s *slot) {
	// store self goroutine pointer
	s.threadPtr = GetG()
	for {
		// exec task
		s.task()
		// notify availability by pushing self reference into stack
		self.push(s)
		// park and wait for call
		mcall(fast_park)
	}
}

// global memory pool for all items used in Pool
var (
	itemPool  = sync.Pool{New: func() any { return new(node) }}
	itemAlloc = itemPool.Get
	itemFree  = itemPool.Put
)

// internal lock-free stack implementation for parking and waking up goroutines
// Credits -> https://github.com/golang-design/lockfree

// a single node in this stack
type node struct {
	next  atomic.Pointer[node]
	value *slot
}

// pop pops value from the top of the stack
func (self *Q[T]) pop() (value *slot) {
	var top, next *node
	for {
		top = self.top.Load()
		if top == nil {
			return
		}
		next = top.next.Load()
		if self.top.CompareAndSwap(top, next) {
			value = top.value
			top.value = nil
			top.next.Store(nil)
			itemFree(top)
			return
		}
	}
}

// push pushes a value on top of the stack
func (self *Q[T]) push(v *slot) {
	var (
		top  *node
		item = itemAlloc().(*node)
	)
	item.value = v
	for {
		top = self.top.Load()
		item.next.Store(top)
		if self.top.CompareAndSwap(top, item) {
			return
		}
	}
}
