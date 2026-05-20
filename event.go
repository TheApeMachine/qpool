package qpool

import (
	"time"

	"github.com/phuslu/log"
)

/*
Field is a key-value pair.
*/
type Field struct {
	Key   string
	Value any
}

/*
Event is a telemetry event.
*/
type Event struct {
	Component string
	Op        string
	Message   string
	Time      time.Time
	Level     log.Level
	Err       error
	Fields    []Field
}

/*
NewEvent creates a new event.
*/
func NewEvent(
	component,
	op,
	message string,
	level log.Level,
	err error,
	fields []Field,
) Event {
	return Event{
		Component: component,
		Op:        op,
		Message:   message,
		Level:     level,
		Err:       err,
		Fields:    fields,
	}
}

/*
NewErrorEvent creates a new error event.
*/
func NewErrorEvent(
	component, op, message string, err error, fields []Field,
) Event {
	return NewEvent(
		component, op, message, log.ErrorLevel, err, fields,
	)
}

/*
NewWarningEvent creates a new warning event.
*/
func NewWarningEvent(
	component, op, message string, fields []Field,
) Event {
	return NewEvent(
		component, op, message, log.WarnLevel, nil, fields,
	)
}

/*
NewInfoEvent creates a new info event.
*/
func NewInfoEvent(
	component, op, message string, fields []Field,
) Event {
	return NewEvent(
		component, op, message, log.InfoLevel, nil, fields,
	)
}

/*
NewDebugEvent creates a new debug event.
*/
func NewDebugEvent(
	component, op, message string, fields []Field,
) Event {
	return NewEvent(
		component, op, message, log.DebugLevel, nil, fields,
	)
}

/*
NewTraceEvent creates a new trace event.
*/
func NewTraceEvent(
	component, op, message string, fields []Field,
) Event {
	return NewEvent(
		component, op, message, log.TraceLevel, nil, fields,
	)
}

/*
NewFatalEvent creates a new fatal event.
*/
func NewFatalEvent(
	component, op, message string, err error, fields []Field,
) Event {
	return NewEvent(
		component, op, message, log.FatalLevel, err, fields,
	)
}

/*
NewPanicEvent creates a new panic event.
*/
func NewPanicEvent(
	component, op, message string, err error, fields []Field,
) Event {
	return NewEvent(
		component, op, message, log.PanicLevel, err, fields,
	)
}

/*
WithFields adds fields to the event.
*/
func (event *Event) WithFields(fields ...Field) *Event {
	event.Fields = append(event.Fields, fields...)
	return event
}

/*
WithField adds a field to the event.
*/
func (event *Event) WithField(key string, value any) *Event {
	event.Fields = append(event.Fields, Field{Key: key, Value: value})
	return event
}

/*
WithError sets the error and upgrades the event level to error.
*/
func (event *Event) WithError(err error) *Event {
	event.Err = err
	event.Level = log.ErrorLevel
	return event
}

/*
WithMessage sets the message.
*/
func (event *Event) WithMessage(message string) *Event {
	event.Message = message
	return event
}

/*
WithLevel sets the level.
*/
func (event *Event) WithLevel(level log.Level) *Event {
	event.Level = level
	return event
}

/*
WithTime sets the time.
*/
func (event *Event) WithTime(time time.Time) *Event {
	event.Time = time
	return event
}

/*
WithComponent sets the component.
*/
func (event *Event) WithComponent(component string) *Event {
	event.Component = component
	return event
}

/*
WithOp sets the operation.
*/
func (event *Event) WithOp(op string) *Event {
	event.Op = op
	return event
}

/*
keyValues.
*/
func (event Event) keyValues() []any {
	pairs := make([]any, 0, len(event.Fields)*2)

	for _, field := range event.Fields {
		pairs = append(pairs, field.Key, field.Value)
	}

	return pairs
}
