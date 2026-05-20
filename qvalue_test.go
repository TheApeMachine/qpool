package qpool

import (
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewQValue(t *testing.T) {
	Convey("Given NewQValue", t, func() {
		before := time.Now()

		value := NewQValue("payload")

		So(value, ShouldNotBeNil)
		So(value.Value, ShouldEqual, "payload")
		So(value.CreatedAt.After(before.Add(-time.Millisecond)), ShouldBeTrue)
		So(value.CreatedAt.Before(time.Now().Add(time.Millisecond)), ShouldBeTrue)
	})
}

func TestQValueID(t *testing.T) {
	Convey("Given QValue ID", t, func() {
		at := time.Date(2024, 5, 10, 9, 0, 0, 123456789, time.UTC)

		value := &QValue{
			Value:     "answer",
			CreatedAt: at,
		}

		id := value.ID()

		So(strings.HasPrefix(id, "qv_answer_"), ShouldBeTrue)
		So(id, ShouldContainSubstring, strconv.FormatInt(at.UnixNano(), 10))
	})
}

func BenchmarkNewQValue(b *testing.B) {
	for range b.N {
		_ = NewQValue(42)
	}
}
