package qpool

import (
	"errors"
	"testing"
	"time"

	"github.com/phuslu/log"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewEvent(t *testing.T) {
	Convey("Given NewEvent", t, func() {
		baseErr := errors.New("e")
		fields := []Field{{Key: "k", Value: 1}}

		event := NewEvent("c", "op", "msg", log.InfoLevel, baseErr, fields)

		So(event.Component, ShouldEqual, "c")
		So(event.Op, ShouldEqual, "op")
		So(event.Message, ShouldEqual, "msg")
		So(event.Level, ShouldEqual, log.InfoLevel)
		So(event.Err, ShouldEqual, baseErr)
		So(event.Fields, ShouldResemble, fields)
	})
}

func TestNewErrorEvent(t *testing.T) {
	Convey("Given NewErrorEvent", t, func() {
		baseErr := errors.New("boom")

		event := NewErrorEvent("svc", "run", "failed", baseErr, nil)

		So(event.Level, ShouldEqual, log.ErrorLevel)
		So(event.Err, ShouldEqual, baseErr)
		So(event.Message, ShouldEqual, "failed")
	})
}

func TestNewWarningEvent(t *testing.T) {
	Convey("Given NewWarningEvent", t, func() {
		event := NewWarningEvent("svc", "run", "slow", nil)

		So(event.Level, ShouldEqual, log.WarnLevel)
		So(event.Err, ShouldBeNil)
	})
}

func TestNewInfoEvent(t *testing.T) {
	Convey("Given NewInfoEvent", t, func() {
		event := NewInfoEvent("svc", "run", "ok", nil)

		So(event.Level, ShouldEqual, log.InfoLevel)
	})
}

func TestNewDebugEvent(t *testing.T) {
	Convey("Given NewDebugEvent", t, func() {
		event := NewDebugEvent("svc", "run", "detail", nil)

		So(event.Level, ShouldEqual, log.DebugLevel)
	})
}

func TestNewTraceEvent(t *testing.T) {
	Convey("Given NewTraceEvent", t, func() {
		event := NewTraceEvent("svc", "run", "fine", nil)

		So(event.Level, ShouldEqual, log.TraceLevel)
	})
}

func TestNewFatalEvent(t *testing.T) {
	Convey("Given NewFatalEvent", t, func() {
		baseErr := errors.New("fatal")

		event := NewFatalEvent("svc", "run", "abort", baseErr, nil)

		So(event.Level, ShouldEqual, log.FatalLevel)
		So(event.Err, ShouldEqual, baseErr)
	})
}

func TestNewPanicEvent(t *testing.T) {
	Convey("Given NewPanicEvent", t, func() {
		baseErr := errors.New("panic")

		event := NewPanicEvent("svc", "run", "panic", baseErr, nil)

		So(event.Level, ShouldEqual, log.PanicLevel)
		So(event.Err, ShouldEqual, baseErr)
	})
}

func TestEventWithFields(t *testing.T) {
	Convey("Given Event WithFields", t, func() {
		event := NewInfoEvent("a", "b", "m", nil)

		ptr := &event
		ptr.WithFields(Field{Key: "x", Value: 2}, Field{Key: "y", Value: 3})

		So(len(event.Fields), ShouldEqual, 2)
		So(event.Fields[0].Key, ShouldEqual, "x")
		So(event.Fields[1].Value, ShouldEqual, 3)
	})
}

func TestEventWithField(t *testing.T) {
	Convey("Given Event WithField", t, func() {
		event := NewInfoEvent("a", "b", "m", nil)

		ptr := &event
		ptr.WithField("rid", "7")

		So(len(event.Fields), ShouldEqual, 1)
		So(event.Fields[0].Key, ShouldEqual, "rid")
		So(event.Fields[0].Value, ShouldEqual, "7")
	})
}

func TestEventWithError(t *testing.T) {
	Convey("Given Event WithError", t, func() {
		event := NewInfoEvent("a", "b", "m", nil)

		baseErr := errors.New("x")
		ptr := &event
		ptr.WithError(baseErr)

		So(event.Err, ShouldEqual, baseErr)
		So(event.Level, ShouldEqual, log.ErrorLevel)
	})
}

func TestEventWithMessage(t *testing.T) {
	Convey("Given Event WithMessage", t, func() {
		event := NewInfoEvent("a", "b", "old", nil)

		ptr := &event
		ptr.WithMessage("new")

		So(event.Message, ShouldEqual, "new")
	})
}

func TestEventWithLevel(t *testing.T) {
	Convey("Given Event WithLevel", t, func() {
		event := NewInfoEvent("a", "b", "m", nil)

		ptr := &event
		ptr.WithLevel(log.WarnLevel)

		So(event.Level, ShouldEqual, log.WarnLevel)
	})
}

func TestEventWithTime(t *testing.T) {
	Convey("Given Event WithTime", t, func() {
		event := NewInfoEvent("a", "b", "m", nil)
		at := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)

		ptr := &event
		ptr.WithTime(at)

		So(event.Time, ShouldEqual, at)
	})
}

func TestEventWithComponent(t *testing.T) {
	Convey("Given Event WithComponent", t, func() {
		event := NewInfoEvent("a", "b", "m", nil)

		ptr := &event
		ptr.WithComponent("pool")

		So(event.Component, ShouldEqual, "pool")
	})
}

func TestEventWithOp(t *testing.T) {
	Convey("Given Event WithOp", t, func() {
		event := NewInfoEvent("a", "b", "m", nil)

		ptr := &event
		ptr.WithOp("schedule")

		So(event.Op, ShouldEqual, "schedule")
	})
}

func BenchmarkNewEvent(b *testing.B) {
	fields := []Field{{Key: "k", Value: 1}}

	for range b.N {
		_ = NewEvent("c", "op", "msg", log.InfoLevel, nil, fields)
	}
}

func BenchmarkEventWithFieldChain(b *testing.B) {
	for range b.N {
		event := NewInfoEvent("c", "op", "msg", nil)

		ptr := &event
		ptr.WithField("a", 1).WithField("b", 2).WithMessage("x")
	}
}
