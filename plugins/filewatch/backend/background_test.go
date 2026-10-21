package main

import (
	"context"
	"errors"
	"iter"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	filewatch "github.com/Tangerg/flame/plugins/filewatch/backend/domain"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/protocol"
)

type watchedFixture struct {
	generation string
	events     chan protocol.RuntimeEvent
}

type fixtureHost struct {
	store              *store
	mu                 sync.Mutex
	admitErr, watchErr error
	watches            chan watchedFixture
	attempts           chan filewatch.Capture
	joined             atomic.Int32
}

func (h *fixtureHost) Watch(ctx context.Context, generation string, _ filewatch.Trigger) (iter.Seq2[protocol.RuntimeEvent, error], error) {
	h.mu.Lock()
	err := h.watchErr
	h.mu.Unlock()
	watch := watchedFixture{generation: generation, events: make(chan protocol.RuntimeEvent, 4)}
	h.watches <- watch
	if err != nil {
		return nil, err
	}
	return func(yield func(protocol.RuntimeEvent, error) bool) {
		defer h.joined.Add(1)
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-watch.events:
				if !yield(event, nil) {
					return
				}
			}
		}
	}, nil
}

func (h *fixtureHost) Admit(ctx context.Context, _ string, observation filewatch.Observation) error {
	capture := observation.Capture()
	invocation, err := pluginsdk.InvocationFromContext(ctx)
	if err != nil || invocation != capture.Invocation {
		return errors.New("fixture: calling owner changed")
	}
	pending, err := h.store.pending(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, event := range pending {
		if reflect.DeepEqual(event.Capture, capture) {
			found = true
		}
	}
	if !found {
		return errors.New("fixture: Run call preceded durable intent")
	}
	h.attempts <- capture
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.admitErr
}

func (*fixtureHost) Notify(context.Context, string) error { return nil }

func receiveFixture[T any](t *testing.T, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("background operation did not arrive")
		var zero T
		return zero
	}
}

func TestBackgroundRetainsOriginalIntentAcrossLostReplyDisableAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "filewatch.db")
	s, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	draft := fixtureDraft()
	configureFixture(t, s, 1, &draft)
	host := &fixtureHost{store: s, admitErr: errors.New("fixture: acknowledgement lost"), watches: make(chan watchedFixture, 4), attempts: make(chan filewatch.Capture, 8)}
	target := capability.Target{Scope: "installation", Reference: "11111111-1111-4111-8111-111111111111"}
	worker := newBackground(t.Context(), s, host, "original-runtime", target, "filewatch-operations", func() error { return nil })
	select {
	case <-host.watches:
		t.Fatal("setup originated business work")
	default:
	}
	if err := worker.Start(); err != nil {
		t.Fatal(err)
	}
	watch := receiveFixture(t, host.watches)
	watch.events <- protocol.RuntimeEvent{Type: protocol.RuntimeFilesChanged, Sequence: 1, WatchID: watch.generation, Workspace: &protocol.WorkspaceRef{Path: draft.Workspace}, Paths: []string{"input.txt"}}
	original := receiveFixture(t, host.attempts)
	if err := worker.Close(); err != nil {
		t.Fatal(err)
	}
	if host.joined.Load() != 1 {
		t.Fatal("closing the activation did not join observation")
	}
	if err := worker.Close(); err != nil {
		t.Fatal(err)
	}
	if err := worker.Start(); err == nil {
		t.Fatal("closed activation restarted")
	}
	configureFixture(t, s, 2, nil)
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	host.store = s
	host.mu.Lock()
	host.admitErr = nil
	host.mu.Unlock()
	worker = newBackground(t.Context(), s, host, "original-runtime", target, "filewatch-operations", func() error { return nil })
	if err := worker.Start(); err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	recovered := receiveFixture(t, host.attempts)
	if !reflect.DeepEqual(original, recovered) || recovered.Invocation.Origin != capability.Automatic || recovered.Invocation.Target != target {
		t.Fatalf("recovery changed captured authority or input: %+v %+v", original, recovered)
	}
	deadline := time.After(5 * time.Second)
	for {
		pending, err := s.pending(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("confirmed intent was not settled")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if worker.snapshot().Phase != filewatch.Disabled {
		t.Fatal("disabled trigger resumed filesystem observation")
	}
}

func TestSourceFailureRequiresRearmAndNeverBecomesANoopWatch(t *testing.T) {
	s, err := openStore(t.Context(), filepath.Join(t.TempDir(), "filewatch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	draft := fixtureDraft()
	configureFixture(t, s, 1, &draft)
	host := &fixtureHost{store: s, watchErr: errors.New("fixture: source unavailable"), watches: make(chan watchedFixture, 4), attempts: make(chan filewatch.Capture, 4)}
	worker := newBackground(t.Context(), s, host, "runtime", capability.Target{Scope: "installation", Reference: "11111111-1111-4111-8111-111111111111"}, "filewatch-operations", func() error { return nil })
	if err := worker.Start(); err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	receiveFixture(t, host.watches)
	select {
	case <-host.watches:
		t.Fatal("failed source was blindly retried")
	case <-time.After(1200 * time.Millisecond):
	}
	if status := worker.snapshot(); status.Phase != filewatch.Failed || status.Error == "" {
		t.Fatalf("source failure became active success: %+v", status)
	}
	configureFixture(t, s, 2, &draft)
	receiveFixture(t, host.watches)
}

func TestResyncAndGapsCaptureUncertaintyInsteadOfInventingFileWrites(t *testing.T) {
	draft := fixtureDraft()
	configuration, err := filewatch.Initial().Replace(1, &draft)
	if err != nil {
		t.Fatal(err)
	}
	worker := &background{target: capability.Target{Scope: "installation", Reference: "11111111-1111-4111-8111-111111111111"}}
	generation := "22222222-2222-4222-8222-222222222222"
	for _, gap := range []bool{false, true} {
		event := protocol.RuntimeEvent{Type: protocol.RuntimeResync, Sequence: 7, Topics: []protocol.RuntimeTopic{protocol.TopicFilesChanged}, WatchIDs: []string{generation}}
		if gap {
			event.Type = protocol.RuntimeFilesChanged
			event.WatchID = "unrelated"
		}
		observation, relevant, err := worker.captureNotice(configuration, generation, event, gap)
		if err != nil || !relevant || observation.Capture().Kind != filewatch.Resync || !reflect.DeepEqual(observation.Capture().Paths, draft.Paths) {
			t.Fatalf("coverage uncertainty became a concrete write: %+v %v", observation, err)
		}
	}
	if _, relevant, err := worker.captureNotice(configuration, generation, protocol.RuntimeEvent{Type: protocol.RuntimeFilesChanged, Sequence: 1, WatchID: "unrelated"}, false); err != nil || relevant {
		t.Fatal("another watch originated work")
	}
}
