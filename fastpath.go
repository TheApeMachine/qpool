package qpool

import (
	"context"
	"fmt"
	"runtime/debug"
)

/*
ScheduleFast dispatches an independent job through the low-overhead runtime
executor. It returns the result directly on the returned channel and does not
store results in QSpace, publish telemetry, apply retries, dependencies,
regulators, circuit breakers, TTLs, or scaler accounting.
*/
func (q *Q[any]) ScheduleFast(
	ctx context.Context,
	fn func(context.Context) (any, error),
) chan *QValue[any] {
	resultChannel := make(chan *QValue[any], 1)

	if ctx == nil {
		ctx = context.Background()
	}

	if q == nil {
		finishFast(resultChannel, nil, fmt.Errorf("qpool: pool closed"))

		return resultChannel
	}

	if fn == nil {
		finishFast(resultChannel, nil, fmt.Errorf("qpool: nil fast job"))

		return resultChannel
	}

	if err := ctx.Err(); err != nil {
		finishFast(resultChannel, nil, err)

		return resultChannel
	}

	q.shutdownMu.RLock()

	if q.stopping.Load() {
		q.shutdownMu.RUnlock()
		finishFast(resultChannel, nil, fmt.Errorf("qpool: pool closed"))

		return resultChannel
	}

	if err := q.ctx.Err(); err != nil {
		q.shutdownMu.RUnlock()
		finishFast(resultChannel, nil, fmt.Errorf("qpool: pool closed: %w", err))

		return resultChannel
	}

	if q.fastQueue == nil {
		q.shutdownMu.RUnlock()
		finishFast(resultChannel, nil, fmt.Errorf("qpool: disruptor queue unavailable"))

		return resultChannel
	}

	work := fastDisruptorWork{
		ctx:    ctx,
		fn:     fn,
		result: resultChannel,
	}

	err := q.fastQueue.publishFast(ctx, work)
	q.shutdownMu.RUnlock()

	if err != nil {
		finishFast(resultChannel, nil, fmt.Errorf("qpool: schedule fast job: %w", err))
	}

	return resultChannel
}

func finishFast(resultChannel chan *QValue[any], value any, err error) {
	qvalue, qvalueErr := NewQValue("", "", value, 0)
	if qvalueErr != nil {
		err = qvalueErr
	}

	qvalue.Error = err
	resultChannel <- qvalue
	close(resultChannel)
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
