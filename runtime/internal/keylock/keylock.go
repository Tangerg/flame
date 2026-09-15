// Package keylock serializes work by key without a blocking mutex. A waiter
// observes the holder's release signal through its own context, so a caller
// whose request ended stops waiting instead of outliving it, and an unrelated
// key never queues behind another key's work.
package keylock

import (
	"context"
	"slices"
	"sync"
)

// Set owns one exclusion per key. The zero value is not usable; build one with
// [NewSet]. Safe for concurrent use.
type Set struct {
	mu   sync.Mutex
	held map[string]chan struct{}
}

func NewSet() *Set { return &Set{held: make(map[string]chan struct{})} }

// Acquire reserves key until the returned release runs. Release is idempotent.
// A context that ends while waiting returns its cause and reserves nothing.
func (s *Set) Acquire(ctx context.Context, key string) (func(), error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, context.Cause(ctx)
		}
		s.mu.Lock()
		held, busy := s.held[key]
		if !busy {
			released := make(chan struct{})
			s.held[key] = released
			s.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					s.mu.Lock()
					delete(s.held, key)
					s.mu.Unlock()
					close(released)
				})
			}, nil
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-held:
		}
	}
}

// AcquireAll reserves every distinct key in one stable order, so two callers
// naming overlapping key sets cannot deadlock against each other. A context
// that ends part-way releases what it already holds and reserves nothing.
func (s *Set) AcquireAll(ctx context.Context, keys ...string) (func(), error) {
	ordered := slices.Clone(keys)
	slices.Sort(ordered)
	ordered = slices.Compact(ordered)
	releases := make([]func(), 0, len(ordered))
	releaseHeld := func() {
		for _, release := range slices.Backward(releases) {
			release()
		}
	}
	for _, key := range ordered {
		release, err := s.Acquire(ctx, key)
		if err != nil {
			releaseHeld()
			return nil, err
		}
		releases = append(releases, release)
	}
	return releaseHeld, nil
}
