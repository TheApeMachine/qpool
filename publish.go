package qpool

import (
	"github.com/phuslu/log"
	"github.com/theapemachine/errnie"
)

var Publish func(Event)

func init() {
	Publish = func(event Event) {
		defaultBroadcaster.Publish(event)

		if defaultLogController.Suppressed() {
			return
		}

		event.log()
	}
}

func (event Event) log() {
	keyValues := event.keyValues()

	switch event.Level {
	case log.ErrorLevel:
		errnie.Error(event.Err, keyValues...)
	case log.WarnLevel:
		errnie.Warn(event.Message, keyValues...)
	case log.InfoLevel:
		errnie.Info(event.Message, keyValues...)
	case log.DebugLevel:
		errnie.Debug(event.Message, keyValues...)
	case log.TraceLevel:
		errnie.Trace(event.Message, keyValues...)
	}
}
