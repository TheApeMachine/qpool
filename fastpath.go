package qpool

import (
	"context"
	"fmt"
	"runtime/debug"
)

/*
ScheduleFast dispatches an independent job through the low-overhead runtime
executor. It returns the result directly on a lock-free handle and does not
store results in QSpace, publish telemetry, apply retries, dependencies,
regulators, circuit breakers, TTLs, or scaler accounting.
*/
func (q *Q[T]) ScheduleFast(
	ctx context.Context,
	fn func(context.Context) (T, error),
) *ResultWait[T] {
	slot := newResultSlot()

	if ctx == nil {
		ctx = context.Background()
	}

	if q == nil {
		slot.Close()

		return errorResultWait[T](fmt.Errorf("qpool: pool closed"))
	}

	if fn == nil {
		slot.Close()

		return errorResultWait[T](fmt.Errorf("qpool: nil fast job"))
	}

	if err := ctx.Err(); err != nil {
		slot.Close()

		return errorResultWait[T](err)
	}

	if q.stopping.Load() {
		slot.Close()

		return errorResultWait[T](fmt.Errorf("qpool: pool closed"))
	}

	if err := q.ctx.Err(); err != nil {
		slot.Close()

		return errorResultWait[T](fmt.Errorf("qpool: pool closed: %w", err))
	}

	if q.fastQueue == nil {
		slot.Close()

		return errorResultWait[T](fmt.Errorf("qpool: disruptor queue unavailable"))
	}

	work := fastDisruptorWork{
		ctx: ctx,
		fn: func(ctx context.Context) (interface{}, error) {
			return fn(ctx)
		},
		result: slot,
	}

	err := q.fastQueue.publishFast(ctx, work)
	if err != nil {
		slot.Close()

		return errorResultWait[T](fmt.Errorf("qpool: schedule fast job: %w", err))
	}

	return pendingResultWait[T](slot)
}

func finishFast(slot *resultSlot, value erasedAny, err error) {
	if slot == nil {
		return
	}

	qvalue, qvalueErr := NewQValue[erasedAny]("", "", value, 0)
	if qvalueErr != nil {
		err = qvalueErr
	}

	qvalue.Error = err
	slot.Deliver(qvalue)
}

func invokeFastFnOnce(
	ctx context.Context,
	fn func(context.Context) (any, error),
) (res any, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = fmt.Errorf("qpool: panic in fast job: %v\n%s", r, debug.Stack())
		}
	}()

	return fn(ctx)
}
