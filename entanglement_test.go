package qpool

import (
	"sync"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewEntanglement(t *testing.T) {
	Convey("Given parameters for a new entanglement", t, func() {
		id := "test-entanglement"
		jobs := []Job{{ID: "job1"}, {ID: "job2"}}
		ttl := 1 * time.Hour

		Convey("When creating a new entanglement", func() {
			entanglement := NewEntanglement(id, jobs, ttl)

			Convey("Then it should be properly initialized", func() {
				So(entanglement, ShouldNotBeNil)
				So(entanglement.ID, ShouldEqual, id)
				So(entanglement.Jobs, ShouldResemble, jobs)
				So(entanglement.TTL, ShouldEqual, ttl)
				So(entanglement.SharedState, ShouldNotBeNil)
				So(entanglement.stateLedger, ShouldNotBeNil)
				So(len(entanglement.stateLedger), ShouldEqual, 0)
			})
		})
	})
}

func TestUpdateState(t *testing.T) {
	Convey("Given an entanglement", t, func() {
		entanglement := NewEntanglement("test-entanglement", []Job{}, 1*time.Hour)

		Convey("When updating state", func() {
			stateChanges := make([]map[string]any, 0)
			entanglement.OnStateChange = func(old, new map[string]any) {
				stateChanges = append(stateChanges, new)
			}

			entanglement.UpdateState("key1", "value1")
			entanglement.UpdateState("key2", 42)
			entanglement.UpdateState("key1", "updated-value1")

			Convey("Then the shared state should be updated", func() {
				So(entanglement.SharedState["key1"], ShouldEqual, "updated-value1")
				So(entanglement.SharedState["key2"], ShouldEqual, 42)
			})

			Convey("Then the state ledger should record all changes", func() {
				So(len(entanglement.stateLedger), ShouldEqual, 3)
				So(entanglement.stateLedger[0].Key, ShouldEqual, "key1")
				So(entanglement.stateLedger[0].Value, ShouldEqual, "value1")
				So(entanglement.stateLedger[1].Key, ShouldEqual, "key2")
				So(entanglement.stateLedger[1].Value, ShouldEqual, 42)
				So(entanglement.stateLedger[2].Key, ShouldEqual, "key1")
				So(entanglement.stateLedger[2].Value, ShouldEqual, "updated-value1")
			})

			Convey("Then the OnStateChange callback should be triggered for each change", func() {
				So(len(stateChanges), ShouldEqual, 3)
				So(stateChanges[2]["key1"], ShouldEqual, "updated-value1")
				So(stateChanges[2]["key2"], ShouldEqual, 42)
			})
		})

		Convey("When updating state concurrently", func() {
			var wg sync.WaitGroup
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func(val int) {
					defer wg.Done()
					entanglement.UpdateState("concurrent", val)
				}(i)
			}
			wg.Wait()

			Convey("Then all updates should be recorded in order", func() {
				history := entanglement.GetStateHistory(0)
				So(len(history), ShouldEqual, 10)
				for i := 0; i < len(history)-1; i++ {
					So(history[i].Sequence, ShouldBeLessThan, history[i+1].Sequence)
				}
			})
		})
	})
}

func TestGetStateHistory(t *testing.T) {
	Convey("Given an entanglement with state history", t, func() {
		entanglement := NewEntanglement("test-entanglement", []Job{}, 1*time.Hour)
		entanglement.UpdateState("key1", "value1")
		entanglement.UpdateState("key2", "value2")
		entanglement.UpdateState("key3", "value3")

		Convey("When getting complete history", func() {
			history := entanglement.GetStateHistory(0)

			Convey("Then all changes should be returned in order", func() {
				So(len(history), ShouldEqual, 3)
				So(history[0].Key, ShouldEqual, "key1")
				So(history[1].Key, ShouldEqual, "key2")
				So(history[2].Key, ShouldEqual, "key3")
			})
		})

		Convey("When getting partial history", func() {
			history := entanglement.GetStateHistory(1)

			Convey("Then only changes after sequence should be returned", func() {
				So(len(history), ShouldEqual, 2)
				So(history[0].Key, ShouldEqual, "key2")
				So(history[1].Key, ShouldEqual, "key3")
			})
		})

		Convey("When getting history with invalid sequence", func() {
			history := entanglement.GetStateHistory(999)

			Convey("Then empty slice should be returned", func() {
				So(len(history), ShouldEqual, 0)
			})
		})
	})
}

func TestReplayStateChanges(t *testing.T) {
	Convey("Given an entanglement with state history", t, func() {
		entanglement := NewEntanglement("test-entanglement", []Job{}, 1*time.Hour)
		
		Convey("When replaying state changes", func() {
			// Create some state history first
			entanglement.UpdateState("key1", "value1")
			entanglement.UpdateState("key2", "value2")
			entanglement.UpdateState("key1", "updated-value1")

			// Set up callback for replay
			stateChanges := make([]map[string]any, 0)
			entanglement.OnStateChange = func(old, new map[string]any) {
				spew.Dump(new)
				stateChanges = append(stateChanges, new)
			}

			// Replay for a new job
			job := Job{ID: "test-job"}
			entanglement.ReplayStateChanges(job)

			Convey("Then all state changes should be replayed in order", func() {
				So(len(stateChanges), ShouldEqual, 3)
				So(stateChanges[0]["key1"], ShouldEqual, "value1")
				So(stateChanges[1]["key2"], ShouldEqual, "value2")
				So(stateChanges[2]["key1"], ShouldEqual, "updated-value1")
			})

			Convey("Then final state should reflect all changes", func() {
				value, exists := entanglement.GetState("key1")
				So(exists, ShouldBeTrue)
				So(value, ShouldEqual, "updated-value1")

				value, exists = entanglement.GetState("key2")
				So(exists, ShouldBeTrue)
				So(value, ShouldEqual, "value2")
			})
		})
	})
}

func TestGetState(t *testing.T) {
	Convey("Given an entanglement with state", t, func() {
		entanglement := NewEntanglement("test-entanglement", []Job{}, 1*time.Hour)
		entanglement.UpdateState("existing-key", "test-value")

		Convey("When getting an existing state key", func() {
			value, exists := entanglement.GetState("existing-key")

			Convey("Then the value should be returned", func() {
				So(exists, ShouldBeTrue)
				So(value, ShouldEqual, "test-value")
			})
		})

		Convey("When getting a non-existent state key", func() {
			value, exists := entanglement.GetState("non-existent-key")

			Convey("Then it should indicate non-existence", func() {
				So(exists, ShouldBeFalse)
				So(value, ShouldBeNil)
			})
		})
	})
}

func TestAddJob(t *testing.T) {
	Convey("Given an entanglement", t, func() {
		entanglement := NewEntanglement("test-entanglement", []Job{}, 1*time.Hour)

		Convey("When adding jobs", func() {
			job1 := Job{ID: "job1"}
			job2 := Job{ID: "job2"}

			entanglement.AddJob(job1)
			entanglement.AddJob(job2)

			Convey("Then the jobs should be added to the entanglement", func() {
				So(len(entanglement.Jobs), ShouldEqual, 2)
				So(entanglement.Jobs[0].ID, ShouldEqual, "job1")
				So(entanglement.Jobs[1].ID, ShouldEqual, "job2")
			})

			Convey("Then LastModified should be updated", func() {
				So(time.Since(entanglement.LastModified), ShouldBeLessThan, time.Second)
			})
		})
	})
}

func TestRemoveJob(t *testing.T) {
	Convey("Given an entanglement with jobs", t, func() {
		entanglement := NewEntanglement("test-entanglement", []Job{
			{ID: "job1"},
			{ID: "job2"},
			{ID: "job3"},
		}, 1*time.Hour)

		Convey("When removing an existing job", func() {
			success := entanglement.RemoveJob("job2")

			Convey("Then the job should be removed", func() {
				So(success, ShouldBeTrue)
				So(len(entanglement.Jobs), ShouldEqual, 2)
				So(entanglement.Jobs[0].ID, ShouldEqual, "job1")
				So(entanglement.Jobs[1].ID, ShouldEqual, "job3")
			})
		})

		Convey("When removing a non-existent job", func() {
			success := entanglement.RemoveJob("non-existent")

			Convey("Then it should return false", func() {
				So(success, ShouldBeFalse)
				So(len(entanglement.Jobs), ShouldEqual, 3)
			})
		})
	})
}

func TestIsExpired(t *testing.T) {
	Convey("Given an entanglement", t, func() {
		Convey("When TTL is positive", func() {
			entanglement := NewEntanglement("test-entanglement", []Job{}, 100*time.Millisecond)

			Convey("Then it should not be expired immediately", func() {
				So(entanglement.IsExpired(), ShouldBeFalse)
			})

			Convey("Then it should be expired after TTL", func() {
				time.Sleep(150 * time.Millisecond)
				So(entanglement.IsExpired(), ShouldBeTrue)
			})

			Convey("Then updating state should reset expiration", func() {
				time.Sleep(50 * time.Millisecond)
				entanglement.UpdateState("key", "value")
				So(entanglement.IsExpired(), ShouldBeFalse)
			})
		})

		Convey("When TTL is zero or negative", func() {
			entanglement := NewEntanglement("test-entanglement", []Job{}, 0)

			Convey("Then it should never expire", func() {
				So(entanglement.IsExpired(), ShouldBeFalse)
				time.Sleep(50 * time.Millisecond)
				So(entanglement.IsExpired(), ShouldBeFalse)
			})
		})
	})
}
