package delivery

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

type subscriptionFileWatcher struct {
	closeErr    error
	duringClose error
	closed      atomic.Int32
	report      func(error)
	notify      func(workspaceapp.ObservationChange)
}

func (w *subscriptionFileWatcher) Watch(_ []workspaceapp.WatchScope, notify func(workspaceapp.ObservationChange), report func(error)) (io.Closer, error) {
	w.report = report
	w.notify = notify
	return w, nil
}

func (w *subscriptionFileWatcher) Close() error {
	if w.duringClose != nil {
		w.report(w.duringClose)
	}
	w.closed.Add(1)
	return w.closeErr
}

type subscriptionAuthoredWatcher struct {
	startupErr error
	closeErr   error
	closed     atomic.Int32
}

func (w *subscriptionAuthoredWatcher) Watch([]string, []workspaceapp.AuthoredResource, func(workspaceapp.AuthoredResource), func(error)) (workspaceapp.AuthoredObservation, error) {
	if w.startupErr != nil {
		return nil, w.startupErr
	}
	return w, nil
}

func (*subscriptionAuthoredWatcher) Accept([]workspaceapp.AuthoredChange) error { return nil }

func (w *subscriptionAuthoredWatcher) Close() error {
	w.closed.Add(1)
	return w.closeErr
}

func TestRuntimeSubscriptionStartupPreservesCleanupFailure(t *testing.T) {
	startup := errors.New("authored observation startup failed")
	cleanup := errors.New("file observation cleanup failed")
	watcher := &subscriptionFileWatcher{closeErr: cleanup}
	handler := newWorkspaceHandler(t.TempDir())
	handler.workspaceHub = newWorkspaceHub()
	handler.workspaceWatch = watcher
	handler.workspaceAuthoredWatch = &subscriptionAuthoredWatcher{startupErr: startup}
	ack, stream, err := handler.SubscribeRuntime(t.Context(), protocol.RuntimeSubscribeRequest{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged, protocol.TopicHooksChanged},
		Watches: []protocol.WatchSpec{{WatchID: "files", Workspace: protocol.WorkspaceRef{Path: t.TempDir()}, Paths: []string{"."}}},
	})
	if ack != nil || stream != nil || !errors.Is(err, startup) || !errors.Is(err, cleanup) {
		t.Fatalf("startup discarded original failures: ack=%v stream-present=%t error=%v", ack, stream != nil, err)
	}
	if count := watcher.closed.Load(); count != 1 {
		t.Fatalf("file observation closed %d times", count)
	}
}

func TestRuntimeSubscriptionRetainsBothObservationCleanupFailures(t *testing.T) {
	root := t.TempDir()
	files := &subscriptionFileWatcher{closeErr: errors.New("file observation cleanup failed")}
	authored := &subscriptionAuthoredWatcher{closeErr: errors.New("authored observation cleanup failed")}
	handler := newWorkspaceHandler(root)
	handler.workspaceHub = newWorkspaceHub()
	handler.workspaceWatch, handler.workspaceAuthoredWatch = files, authored
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, stream, err := handler.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged, protocol.TopicHooksChanged},
		Watches: []protocol.WatchSpec{{WatchID: "files", Workspace: protocol.WorkspaceRef{Path: root}, Paths: []string{"."}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	var terminal []error
	for _, err := range stream {
		if err != nil {
			terminal = append(terminal, err)
		}
	}
	if len(terminal) != 1 || !errors.Is(terminal[0], files.closeErr) || !errors.Is(terminal[0], authored.closeErr) {
		t.Fatalf("cleanup failure discarded an owner: %v", terminal)
	}
	if files.closed.Load() != 1 || authored.closed.Load() != 1 {
		t.Fatalf("observation release counts: files=%d authored=%d", files.closed.Load(), authored.closed.Load())
	}
}

func TestRuntimeSubscriptionSettlesCleanupBeforeTerminalFailure(t *testing.T) {
	observation := errors.New("file observation failed")
	for _, test := range []struct {
		name        string
		beforeClose error
		duringClose error
	}{
		{name: "observation failure", beforeClose: observation},
		{name: "cancellation"},
		{name: "observation failure during cancellation", duringClose: observation},
	} {
		t.Run(test.name, func(t *testing.T) {
			cleanup := errors.New("file observation cleanup failed")
			watcher := &subscriptionFileWatcher{closeErr: cleanup, duringClose: test.duringClose}
			handler := newWorkspaceHandler(t.TempDir())
			handler.workspaceHub = newWorkspaceHub()
			handler.workspaceWatch = watcher
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			_, stream, err := handler.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
				Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged},
				Watches: []protocol.WatchSpec{{WatchID: "files", Workspace: protocol.WorkspaceRef{Path: t.TempDir()}, Paths: []string{"."}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if test.beforeClose != nil {
				watcher.report(test.beforeClose)
			} else {
				cancel()
			}
			var terminal []error
			for _, err := range stream {
				if err == nil {
					continue
				}
				if count := watcher.closed.Load(); count != 1 {
					t.Errorf("terminal failure preceded cleanup: close count=%d", count)
				}
				terminal = append(terminal, err)
			}
			if len(terminal) != 1 || !errors.Is(errors.Join(terminal...), cleanup) {
				t.Fatalf("cleanup failure became a clean end: %v", terminal)
			}
			if (test.beforeClose != nil || test.duringClose != nil) && !errors.Is(terminal[0], observation) {
				t.Fatalf("original observation failure lost: %v", terminal[0])
			}
			if count := watcher.closed.Load(); count != 1 {
				t.Fatalf("file observation closed %d times", count)
			}
		})
	}
}

func TestRuntimeSubscriptionEarlyStopJoinsCleanupWithoutAnotherYield(t *testing.T) {
	root := t.TempDir()
	watcher := &subscriptionFileWatcher{closeErr: errors.New("file observation cleanup failed")}
	handler := newWorkspaceHandler(root)
	handler.workspaceHub = newWorkspaceHub()
	handler.workspaceWatch = watcher
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, stream, err := handler.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics:  []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		Watches: []protocol.WatchSpec{{WatchID: "files", Workspace: protocol.WorkspaceRef{Path: root}, Paths: []string{"."}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	watcher.notify(workspaceapp.ObservationChange{Key: "files", Root: root, Paths: []string{"."}})
	for event, err := range stream {
		if err != nil || event.WatchID != "files" {
			t.Fatalf("original notification changed: %+v %v", event, err)
		}
		break
	}
	if count := watcher.closed.Load(); count != 1 {
		t.Fatalf("early stop closed file observation %d times", count)
	}
}
