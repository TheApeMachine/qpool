package qpool

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQSpaceAwait(test *testing.T) {
	Convey("Given QSpace Await on a pending id", test, func() {
		qspace := NewQSpace(test.Context())
		defer qspace.Close()

		wait := qspace.Await("job")
		qspace.Store("job", "ok", 0)

		Convey("It should deliver one value", func() {
			result, err := wait.Get(context.Background())

			So(err, ShouldBeNil)
			So(result, ShouldNotBeNil)
			So(ArtifactError(result), ShouldBeNil)

			payload := result.DecryptPayload()
			So(string(payload), ShouldEqual, "ok")
		})
	})
}

func TestQSpaceStore(test *testing.T) {
	Convey("Given QSpace Store", test, func() {
		qspace := NewQSpace(test.Context())
		defer qspace.Close()

		qspace.Store("job", "ok", 0)

		Convey("It should persist the stored value", func() {
			result, ok := qspace.PeekResult("job")

			So(ok, ShouldBeTrue)
			So(result, ShouldNotBeNil)

			payload := result.DecryptPayload()
			So(string(payload), ShouldEqual, "ok")
		})
	})
}

func TestQSpaceAddRelationship(test *testing.T) {
	Convey("Given QSpace dependency relationships", test, func() {
		qspace := NewQSpace(test.Context())
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
