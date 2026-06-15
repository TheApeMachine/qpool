package qpool

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewResultArtifact(t *testing.T) {
	Convey("Given newResultArtifact", t, func() {
		before := time.Now()

		artifact, err := newResultArtifact("job", "payload", time.Minute)

		So(err, ShouldBeNil)
		So(artifact, ShouldNotBeNil)
		So(artifact.Timestamp(), ShouldBeGreaterThanOrEqualTo, before.UnixNano())
	})

	Convey("When value is nil for error-only results", t, func() {
		artifact, err := newErrorArtifact("job", nil, time.Minute)

		So(err, ShouldBeNil)
		So(artifact, ShouldNotBeNil)
		So(ArtifactError(artifact), ShouldNotBeNil)
	})
}

func BenchmarkNewResultArtifact(b *testing.B) {
	for b.Loop() {
		artifact, err := newResultArtifact("job", "payload", time.Minute)

		if err != nil || artifact == nil {
			b.Fatal("newResultArtifact failed")
		}
	}
}
