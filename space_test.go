package qpool

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQuantumSpace(t *testing.T) {
	Convey("Given a quantum space", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()

		qs := newQuantumSpace()

		const key = "test-group"

		Reset(func() {
			qs.Close()
			time.Sleep(cleanupTimeout)
		})

		Convey("When storing and retrieving values", func() {
			qs.Store("test-key", "test-value", nil, time.Minute)

			Convey("Value should be retrievable", func() {
				ch := qs.Await("test-key")
				select {
				case <-ctx.Done():
					t.Fatal("Test timed out waiting for value retrieval")
				case value := <-ch:
					So(value.Value, ShouldEqual, "test-value")
					So(value.Error, ShouldBeNil)
				}
			})
		})

		Convey("When using broadcast groups", func() {
			group := qs.CreateBroadcastGroup(key, time.Minute)
			sub1 := qs.Subscribe(key)
			sub2 := qs.Subscribe(key)

			defer func() {
				close(sub1)
				close(sub2)
			}()

			Convey("All subscribers should receive messages", func() {
				testValue := QuantumValue{Value: "broadcast message", CreatedAt: time.Now()}
				group.Send(testValue)

				for i, ch := range []chan QuantumValue{sub1, sub2} {
					select {
					case <-ctx.Done():
						t.Fatalf("Test timed out waiting for subscriber %d", i+1)
					case msg := <-ch:
						So(msg.Value, ShouldEqual, "broadcast message")
					}
				}
			})
		})
	})
}
