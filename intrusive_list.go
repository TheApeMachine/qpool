package qpool

import "sync/atomic"

/*
IntrusiveList is a lock-free singly linked list rooted at an atomic head pointer.
Head removal and middle-node removal use CAS; bind must supply tryUnlink that
atomically swaps prev's next from current to next.
*/
type IntrusiveList[T any] struct {
	head       atomic.Pointer[T]
	loadNext   func(*T) *T
	linkBefore func(*T, *T)
	tryUnlink  func(prev, current, next *T) bool
}

func (list *IntrusiveList[T]) bind(
	loadNext func(*T) *T,
	linkBefore func(*T, *T),
	tryUnlink func(prev, current, next *T) bool,
) {
	if loadNext == nil || linkBefore == nil || tryUnlink == nil {
		panic("qpool: IntrusiveList.bind requires non-nil loadNext, linkBefore, and tryUnlink")
	}

	list.loadNext = loadNext
	list.linkBefore = linkBefore
	list.tryUnlink = tryUnlink
}

func (list *IntrusiveList[T]) requireBound() {
	if list.loadNext == nil || list.linkBefore == nil || list.tryUnlink == nil {
		panic("qpool: IntrusiveList used before bind")
	}
}

func (list *IntrusiveList[T]) Prepend(node *T) {
	if list == nil || node == nil {
		return
	}

	list.requireBound()

	for {
		if list.prependOnce(node) {
			return
		}
	}
}

func (list *IntrusiveList[T]) prependOnce(node *T) bool {
	head := list.head.Load()
	list.linkBefore(node, head)

	return list.head.CompareAndSwap(head, node)
}

/*
Remove unlinks the first matching node. Safe for concurrent head and middle removal
when bind supplied a CAS-based tryUnlink callback.
*/
func (list *IntrusiveList[T]) Remove(match func(*T) bool) {
	_ = list.RemoveReturning(match)
}

/*
RemoveReturning unlinks the first matching node and returns it, or nil when none matched.
*/
func (list *IntrusiveList[T]) RemoveReturning(match func(*T) bool) *T {
	if list == nil || match == nil {
		return nil
	}

	list.requireBound()

	for {
		retry := false
		var prev *T
		current := list.head.Load()

		for current != nil {
			next := list.loadNext(current)

			if match(current) {
				if prev == nil {
					if !list.head.CompareAndSwap(current, next) {
						retry = true

						break
					}

					return current
				}

				if list.tryUnlink(prev, current, next) {
					return current
				}

				retry = true

				break
			}

			prev = current
			current = next
		}

		if !retry {
			return nil
		}
	}
}

func (list *IntrusiveList[T]) Find(match func(*T) bool) *T {
	if list == nil || match == nil {
		return nil
	}

	list.requireBound()

	for current := list.head.Load(); current != nil; current = list.loadNext(current) {
		if match(current) {
			return current
		}
	}

	return nil
}

func (list *IntrusiveList[T]) PopHead() *T {
	if list == nil {
		return nil
	}

	list.requireBound()

	for {
		head := list.head.Load()

		if head == nil {
			return nil
		}

		next := list.loadNext(head)

		if list.head.CompareAndSwap(head, next) {
			return head
		}
	}
}

func (list *IntrusiveList[T]) Head() *T {
	if list == nil {
		return nil
	}

	return list.head.Load()
}

func (list *IntrusiveList[T]) Clear() {
	if list == nil {
		return
	}

	list.head.Store(nil)
}

func (list *IntrusiveList[T]) Walk(visitor func(*T)) {
	if list == nil || visitor == nil {
		return
	}

	list.requireBound()

	for current := list.head.Load(); current != nil; current = list.loadNext(current) {
		visitor(current)
	}
}
