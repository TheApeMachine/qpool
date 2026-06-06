package qpool

import "sync/atomic"

/*
IntrusiveList is a lock-free singly linked list rooted at an atomic head pointer.
*/
type IntrusiveList[T any] struct {
	head       atomic.Pointer[T]
	loadNext   func(*T) *T
	linkBefore func(*T, *T)
}

func (list *IntrusiveList[T]) bind(
	loadNext func(*T) *T,
	linkBefore func(*T, *T),
) {
	list.loadNext = loadNext
	list.linkBefore = linkBefore
}

func (list *IntrusiveList[T]) Prepend(node *T) {
	if list == nil || node == nil {
		return
	}

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

func (list *IntrusiveList[T]) Remove(match func(*T) bool) {
	if list == nil {
		return
	}

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

					return
				}

				list.linkBefore(prev, next)

				return
			}

			prev = current
			current = next
		}

		if !retry {
			return
		}
	}
}

func (list *IntrusiveList[T]) Find(match func(*T) bool) *T {
	if list == nil {
		return nil
	}

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

	for current := list.head.Load(); current != nil; current = list.loadNext(current) {
		visitor(current)
	}
}
