package bootstrap

import "sync"

type notificationRelay[T any] struct {
	publish func(T)
	observe func(func(T))
}

func newNotificationRelay[T any]() notificationRelay[T] {
	var mu sync.RWMutex
	var sink func(T)
	return notificationRelay[T]{
		publish: func(value T) {
			mu.RLock()
			current := sink
			mu.RUnlock()
			if current != nil {
				current(value)
			}
		},
		observe: func(next func(T)) {
			mu.Lock()
			sink = next
			mu.Unlock()
		},
	}
}
