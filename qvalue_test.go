package qpool

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewQValue(t *testing.T) {
	Convey("Given NewQValue", t, func() {
		before := time.Now()

		value, err := NewQValue[any]("", "", "payload", 0)
		if err != nil {
			t.Fatal(err)
		}

		So(value, ShouldNotBeNil)
		So(value.Value, ShouldEqual, "payload")
		So(time.Unix(0, value.CreatedAt).After(before.Add(-time.Millisecond)), ShouldBeTrue)
		So(time.Unix(0, value.CreatedAt).Before(time.Now().Add(time.Millisecond)), ShouldBeTrue)
	})

	Convey("When Value is nil for error-only results", t, func() {
		value, err := NewQValue[any]("", "", nil, 0)

		So(err, ShouldBeNil)
		So(value, ShouldNotBeNil)
		So(value.Value, ShouldBeNil)
	})
}

func BenchmarkNewQValue(b *testing.B) {
	for b.Loop() {
		_, err := NewQValue[any]("", "", 42, 0)
		if err != nil {
			b.Fatal(err)
		}
	}
}
