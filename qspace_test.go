package qpool

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQSpaceAwait(test *testing.T) {
	Convey("Given QSpace Await on a pending id", test, func() {
		qspace := NewQSpace()
		defer qspace.Close()

		resultChannel := qspace.Await("job")

		qspace.Store("job", "ok", 0)

		Convey("It should deliver one value and close the channel", func() {
			result, open := <-resultChannel

			So(open, ShouldBeTrue)
			So(result, ShouldNotBeNil)
			So(result.Error, ShouldBeNil)
			So(result.Value, ShouldEqual, "ok")

			_, open = <-resultChannel

			So(open, ShouldBeFalse)
		})
	})
}

func TestQSpaceStore(test *testing.T) {
	Convey("Given QSpace Store while the dependency graph lock is busy", test, func() {
		qspace := NewQSpace()
		defer qspace.Close()

		qspace.graphMu.Lock()

		done := make(chan struct{})

		go func() {
			qspace.Store("job", "ok", 0)
			close(done)
		}()

		Convey("It should not block result storage on graph traversal work", func() {
			select {
			case <-done:
			case <-time.After(100 * time.Millisecond):
				qspace.graphMu.Unlock()
				test.Fatal("Store blocked on the graph lock")
			}

			qspace.graphMu.Unlock()

			result, ok := qspace.PeekResult("job")

			So(ok, ShouldBeTrue)
			So(result, ShouldNotBeNil)
			So(result.Value, ShouldEqual, "ok")
		})
	})
}

func TestQSpaceAddRelationship(test *testing.T) {
	Convey("Given QSpace dependency relationships", test, func() {
		qspace := NewQSpace()
		defer qspace.Close()

		So(qspace.AddRelationship("a", "b"), ShouldBeNil)
		So(qspace.AddRelationship("b", "c"), ShouldBeNil)

		Convey("It should reject a circular edge", func() {
			err := qspace.AddRelationship("c", "a")

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "circular")
		})
	})
}
