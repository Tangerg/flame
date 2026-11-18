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

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	filewatch "github.com/Tangerg/flame/plugins/filewatch/internal/domain"

	framecall "github.com/Tangerg/frame/call"
	"github.com/google/uuid"
)

type host interface {
	Watch(context.Context, string, filewatch.Trigger) (iter.Seq2[protocol.RuntimeEvent, error], error)
	Admit(context.Context, string, filewatch.Observation) error
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

func openStream(ctx context.Context, source host, generation string, trigger filewatch.Trigger) (*observationStream, error) {
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

// watcher follows the configured trigger's Runtime file events from
// activation until Close and admits each captured observation as a Run.
type watcher struct {
	mu           sync.Mutex
	status       filewatch.Status
	store        *store
	host         host
	namespace    string
	target       framecall.Target
	contribution string
}

func newWatcher(store *store, host host, namespace string, target framecall.Target, contribution string) *watcher {
	return &watcher{store: store, host: host, namespace: namespace, target: target, contribution: contribution, status: filewatch.Status{Phase: filewatch.Inactive}}
}

func (w *watcher) background(lifetime context.Context, release func() error) (*pluginmcp.Background, error) {
	return pluginmcp.NewBackground(lifetime, pluginmcp.BackgroundConfig{
		Prepare: func(ctx context.Context) error { return pluginstorage.BindRuntimeTarget(ctx, w.store.db, w.namespace) },
		Run:     w.run,
		Release: release,
	})
}

func (w *watcher) snapshot() filewatch.Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

func (w *watcher) observationStatus(revision int64, phase filewatch.Phase, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.status.Revision, w.status.Phase, w.status.Error = revision, phase, ""
	if err != nil {
		w.status.Error = err.Error()
	}
}

func (w *watcher) dispatchStatus(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.status.DispatchError = ""
	if err != nil {
		w.status.DispatchError = err.Error()
	}
}

func (w *watcher) invocation() framecall.Invocation {
	return framecall.Invocation{ID: uuid.NewString(), Origin: extension.Automatic, Target: w.target}
}

func (w *watcher) run(ctx context.Context) {
	defer w.observationStatus(0, filewatch.Inactive, nil)
	var stream *observationStream
	defer func() { stream.close() }()
	var notices <-chan observationNotice
	var current filewatch.Configuration
	var generation string
	var cursor filewatch.Cursor
	stop := func(err error) {
		stream.close()
		stream, notices = nil, nil
		w.observationStatus(current.Revision(), filewatch.Failed, err)
	}
	refresh := func() {
		configuration, err := w.store.configuration(ctx)
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
			w.observationStatus(current.Revision(), filewatch.Disabled, nil)
			return
		}
		generation = uuid.NewString()
		cursor, err = filewatch.NewCursor(generation)
		if err != nil {
			stop(err)
			return
		}
		invocation, err := pluginmcp.WithInvocation(ctx, w.invocation())
		if err == nil {
			stream, err = openStream(invocation, w.host, generation, trigger)
		}
		if err != nil {
			stop(err)
			return
		}
		notices = stream.notices
		w.observationStatus(current.Revision(), filewatch.Watching, nil)
	}
	refresh()
	w.reconcile(ctx)
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			refresh()
			w.reconcile(ctx)
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
			observation, relevant, err := w.captureNotice(current, generation, notice.event, gap)
			if err != nil {
				stop(err)
				continue
			}
			if !relevant {
				continue
			}
			if err := w.store.capture(ctx, observation); err != nil {
				if errors.Is(err, filewatch.ErrRevision) {
					refresh()
				} else {
					stop(err)
				}
				continue
			}
			w.notify(ctx)
			w.reconcile(ctx)
		}
	}
}

func (w *watcher) captureNotice(configuration filewatch.Configuration, generation string, event protocol.RuntimeEvent, gap bool) (filewatch.Observation, bool, error) {
	trigger, enabled := configuration.Trigger()
	if !enabled {
		return filewatch.Observation{}, false, filewatch.ErrInvalid
	}
	draft := trigger.Draft()
	kind, paths := filewatch.Changed, event.Paths
	switch {
	case gap:
		kind, paths = filewatch.Resync, draft.Paths
	case event.Type == protocol.RuntimeResync:
		if !slices.Contains(event.Topics, protocol.TopicFilesChanged) || (len(event.WatchIDs) > 0 && !slices.Contains(event.WatchIDs, generation)) {
			return filewatch.Observation{}, false, nil
		}
		kind, paths = filewatch.Resync, draft.Paths
	case event.Type == protocol.RuntimeFilesChanged && event.WatchID == generation:
		if event.Workspace == nil || event.Workspace.Path != draft.Workspace {
			return filewatch.Observation{}, false, errors.New("filewatch: observation workspace differs from configured source")
		}
	default:
		return filewatch.Observation{}, false, nil
	}
	observation, err := filewatch.NewObservation(filewatch.Capture{Generation: generation, Sequence: event.Sequence, Revision: configuration.Revision(), Kind: kind, Paths: paths, ObservedAt: time.Now(), Trigger: draft,
		SessionID: identity.SessionPrefix + uuid.NewString(), RunID: identity.RunPrefix + uuid.NewString(), Invocation: w.invocation()})
	return observation, true, err
}

func (w *watcher) reconcile(ctx context.Context) {
	events, err := w.store.pending(ctx)
	if err != nil {
		w.dispatchStatus(err)
		return
	}
	var failures error
	for _, event := range events {
		if ctx.Err() != nil {
			return
		}
		observation, err := filewatch.NewObservation(event.Capture)
		if err == nil && event.Capture.Invocation.Target != w.target {
			err = errors.New("filewatch: capture belongs to another installation")
		}
		if err == nil {
			admission, cancel := context.WithTimeout(ctx, 30*time.Second)
			admission, err = pluginmcp.WithInvocation(admission, event.Capture.Invocation)
			if err == nil {
				err = w.host.Admit(admission, w.namespace, observation)
			}
			if err == nil {
				err = w.store.acceptRun(admission, event.Capture.Invocation.ID)
			}
			cancel()
		}
		if err != nil {
			failures = errors.Join(failures, err)
		} else {
			w.notify(ctx)
		}
	}
	w.dispatchStatus(failures)
}

func (w *watcher) notify(ctx context.Context) {
	invocation, err := pluginmcp.WithInvocation(ctx, w.invocation())
	if err == nil {
		err = w.host.Notify(invocation, w.contribution)
	}
	if err != nil && ctx.Err() == nil {
		slog.ErrorContext(ctx, "filewatch: read invalidation failed", "error", err)
	}
}
