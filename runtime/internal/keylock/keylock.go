package keylock

import (
	"context"
	"errors"
	"slices"
	"sync"
)

type Set struct {
	mu   sync.Mutex
	held map[string]chan struct{}
}

func NewSet() *Set { return &Set{held: make(map[string]chan struct{})} }

func (s *Set) Acquire(ctx context.Context, key string) (func(), error) {
	for {
		s.mu.Lock()
		if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
			s.mu.Unlock()
			return nil, err
		}
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
			return nil, errors.Join(ctx.Err(), context.Cause(ctx))
		case <-held:
		}
	}
}

// A stable order so callers naming overlapping key sets cannot deadlock.
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
