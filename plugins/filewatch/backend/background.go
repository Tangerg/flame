package main

import (
	"context"
	"errors"
	"io"
	"iter"
	"log/slog"
	"slices"
	"sync"
	"time"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	"github.com/Tangerg/flame/plugins/filewatch/backend/domain"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/identity"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/google/uuid"
)

type host interface {
	Watch(context.Context, string, domain.Trigger) (iter.Seq2[protocol.RuntimeEvent, error], error)
	Admit(context.Context, string, domain.Observation) error
	Notify(context.Context, string) error
}

type observationNotice struct {
	event protocol.RuntimeEvent
	err   error
}

type observationStream struct {
	notices <-chan observationNotice
	cancel  context.CancelFunc
	done    <-chan struct{}
}

func openStream(ctx context.Context, source host, generation string, trigger domain.Trigger) (*observationStream, error) {
	lifetime, cancel := context.WithCancel(ctx)
	events, err := source.Watch(lifetime, generation, trigger)
	if err != nil {
		cancel()
		return nil, err
	}
	notices := make(chan observationNotice, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(notices)
		emit := func(value observationNotice) bool {
			select {
			case notices <- value:
				return true
			case <-lifetime.Done():
				return false
			}
		}
		for event, err := range events {
			if !emit(observationNotice{event: event, err: err}) || err != nil {
				return
			}
		}
		if lifetime.Err() == nil {
			emit(observationNotice{err: io.ErrUnexpectedEOF})
		}
	}()
	return &observationStream{notices: notices, cancel: cancel, done: done}, nil
}

func (s *observationStream) close() {
	if s != nil {
		s.cancel()
		<-s.done
	}
}

type background struct {
	ctx             context.Context
	cancel          context.CancelFunc
	done            chan struct{}
	closeDone       chan struct{}
	mu              sync.Mutex
	started, closed bool
	closeErr        error
	status          domain.Status
	store           *store
	host            host
	namespace       string
	target          capability.Target
	contribution    string
	closeHost       func() error
}

func newBackground(ctx context.Context, store *store, host host, namespace string, target capability.Target, contribution string, closeHost func() error) *background {
	lifetime, cancel := context.WithCancel(ctx)
	return &background{ctx: lifetime, cancel: cancel, done: make(chan struct{}), closeDone: make(chan struct{}), store: store, host: host, namespace: namespace, target: target, contribution: contribution, closeHost: closeHost, status: domain.Status{Phase: domain.Inactive}}
}

func (b *background) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.ctx.Err() != nil {
		return errors.New("filewatch: background lifetime is closed")
	}
	if b.started {
		return nil
	}
	if err := b.store.bindNamespace(b.ctx, b.namespace); err != nil {
		return err
	}
	b.started = true
	go b.run()
	return nil
}

func (b *background) Close() error {
	b.cancel()
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		<-b.closeDone
		return b.closeErr
	}
	b.closed = true
	if !b.started {
		close(b.done)
	}
	b.mu.Unlock()
	<-b.done
	b.closeErr = b.closeHost()
	close(b.closeDone)
	return b.closeErr
}

func (b *background) snapshot() domain.Status {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status
}

func (b *background) observationStatus(revision int64, phase domain.Phase, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status.Revision, b.status.Phase, b.status.Error = revision, phase, ""
	if err != nil {
		b.status.Error = err.Error()
	}
}

func (b *background) dispatchStatus(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status.DispatchError = ""
	if err != nil {
		b.status.DispatchError = err.Error()
	}
}

func (b *background) invocation() capability.Invocation {
	return capability.Invocation{ID: uuid.NewString(), Origin: capability.Automatic, Target: b.target}
}

func (b *background) run() {
	defer close(b.done)
	defer b.observationStatus(0, domain.Inactive, nil)
	var stream *observationStream
	defer func() { stream.close() }()
	var notices <-chan observationNotice
	var current domain.Configuration
	var generation string
	var cursor domain.Cursor
	stop := func(err error) {
		stream.close()
		stream, notices = nil, nil
		b.observationStatus(current.Revision(), domain.Failed, err)
	}
	refresh := func() {
		configuration, err := b.store.configuration(b.ctx)
		if err != nil {
			stop(err)
			return
		}
		if configuration.Revision() == current.Revision() {
			return
		}
		stream.close()
		stream, notices = nil, nil
		current = configuration
		trigger, enabled := current.Trigger()
		if !enabled {
			b.observationStatus(current.Revision(), domain.Disabled, nil)
			return
		}
		generation = uuid.NewString()
		cursor, err = domain.NewCursor(generation)
		if err != nil {
			stop(err)
			return
		}
		ctx, err := pluginsdk.WithInvocation(b.ctx, b.invocation())
		if err == nil {
			stream, err = openStream(ctx, b.host, generation, trigger)
		}
		if err != nil {
			stop(err)
			return
		}
		notices = stream.notices
		b.observationStatus(current.Revision(), domain.Watching, nil)
	}
	refresh()
	b.reconcile()
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-timer.C:
			refresh()
			b.reconcile()
		case notice, open := <-notices:
			if !open {
				stop(io.ErrUnexpectedEOF)
				continue
			}
			if notice.err != nil {
				stop(notice.err)
				continue
			}
			gap, err := cursor.Advance(notice.event.Sequence)
			if err != nil {
				stop(err)
				continue
			}
			observation, relevant, err := b.captureNotice(current, generation, notice.event, gap)
			if err != nil {
				stop(err)
				continue
			}
			if !relevant {
				continue
			}
			if err := b.store.capture(b.ctx, observation); err != nil {
				if errors.Is(err, domain.ErrRevision) {
					refresh()
				} else {
					stop(err)
				}
				continue
			}
			b.notify()
			b.reconcile()
		}
	}
}

func (b *background) captureNotice(configuration domain.Configuration, generation string, event protocol.RuntimeEvent, gap bool) (domain.Observation, bool, error) {
	trigger, enabled := configuration.Trigger()
	if !enabled {
		return domain.Observation{}, false, domain.ErrInvalid
	}
	draft := trigger.Draft()
	kind, paths := domain.Changed, event.Paths
	switch {
	case gap:
		kind, paths = domain.Resync, draft.Paths
	case event.Type == protocol.RuntimeResync:
		if !slices.Contains(event.Topics, protocol.TopicFilesChanged) || (len(event.WatchIDs) > 0 && !slices.Contains(event.WatchIDs, generation)) {
			return domain.Observation{}, false, nil
		}
		kind, paths = domain.Resync, draft.Paths
	case event.Type == protocol.RuntimeFilesChanged && event.WatchID == generation:
		if event.Workspace == nil || event.Workspace.Path != draft.Workspace {
			return domain.Observation{}, false, errors.New("filewatch: observation workspace differs from configured source")
		}
	default:
		return domain.Observation{}, false, nil
	}
	observation, err := domain.NewObservation(domain.Capture{Generation: generation, Sequence: event.Sequence, Revision: configuration.Revision(), Kind: kind, Paths: paths, ObservedAt: time.Now(), Trigger: draft,
		SessionID: identity.SessionPrefix + uuid.NewString(), RunID: identity.RunPrefix + uuid.NewString(), Invocation: b.invocation()})
	return observation, true, err
}

func (b *background) reconcile() {
	events, err := b.store.pending(b.ctx)
	if err != nil {
		b.dispatchStatus(err)
		return
	}
	var failures error
	for _, event := range events {
		if b.ctx.Err() != nil {
			return
		}
		observation, err := domain.NewObservation(event.Capture)
		if err == nil && event.Capture.Invocation.Target != b.target {
			err = errors.New("filewatch: capture belongs to another installation")
		}
		if err == nil {
			ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
			ctx, err = pluginsdk.WithInvocation(ctx, event.Capture.Invocation)
			if err == nil {
				err = b.host.Admit(ctx, b.namespace, observation)
			}
			if err == nil {
				err = b.store.acceptRun(ctx, event.Capture.Invocation.ID)
			}
			cancel()
		}
		if err != nil {
			failures = errors.Join(failures, err)
		} else {
			b.notify()
		}
	}
	b.dispatchStatus(failures)
}

func (b *background) notify() {
	ctx, err := pluginsdk.WithInvocation(b.ctx, b.invocation())
	if err == nil {
		err = b.host.Notify(ctx, b.contribution)
	}
	if err != nil && b.ctx.Err() == nil {
		slog.ErrorContext(b.ctx, "filewatch: read invalidation failed", "error", err)
	}
}
