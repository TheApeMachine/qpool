package qpool

import (
	"sync/atomic"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type testListNode struct {
	id   int
	next atomic.Pointer[testListNode]
}

func newTestListNode(id int) *testListNode {
	return &testListNode{id: id}
}

func bindTestListNode(list *IntrusiveList[testListNode]) {
	list.bind(
		func(node *testListNode) *testListNode {
			return node.next.Load()
		},
		func(node, next *testListNode) {
			node.next.Store(next)
		},
	)
}

func TestIntrusiveList_Prepend(test *testing.T) {
	Convey("Given IntrusiveList Prepend", test, func() {
		list := IntrusiveList[testListNode]{}
		bindTestListNode(&list)

		list.Prepend(newTestListNode(1))
		list.Prepend(newTestListNode(2))

		Convey("It should prepend at the head", func() {
			So(list.Head().id, ShouldEqual, 2)
		})
	})
}

func TestIntrusiveList_Remove(test *testing.T) {
	Convey("Given IntrusiveList Remove", test, func() {
		list := IntrusiveList[testListNode]{}
		bindTestListNode(&list)

		list.Prepend(newTestListNode(1))
		list.Prepend(newTestListNode(2))

		list.Remove(func(node *testListNode) bool {
			return node.id == 2
		})

		Convey("It should remove the matching node", func() {
			So(list.Head().id, ShouldEqual, 1)
		})
	})
}

func TestIntrusiveList_Find(test *testing.T) {
	Convey("Given IntrusiveList Find", test, func() {
		list := IntrusiveList[testListNode]{}
		bindTestListNode(&list)

		list.Prepend(newTestListNode(1))

		found := list.Find(func(node *testListNode) bool {
			return node.id == 1
		})

		Convey("It should return the matching node", func() {
			So(found, ShouldNotBeNil)
			So(found.id, ShouldEqual, 1)
		})
	})
}

func TestIntrusiveList_PopHead(test *testing.T) {
	Convey("Given IntrusiveList PopHead", test, func() {
		list := IntrusiveList[testListNode]{}
		bindTestListNode(&list)

		list.Prepend(newTestListNode(1))

		popped := list.PopHead()

		Convey("It should pop the head node", func() {
			So(popped.id, ShouldEqual, 1)
			So(list.Head(), ShouldBeNil)
		})
	})
}

func BenchmarkIntrusiveList_PrependRemove(benchmark *testing.B) {
	list := IntrusiveList[testListNode]{}
	bindTestListNode(&list)
	node := newTestListNode(1)

	for benchmark.Loop() {
		list.Prepend(node)
		list.Remove(func(current *testListNode) bool {
			return current == node
		})
	}
}
