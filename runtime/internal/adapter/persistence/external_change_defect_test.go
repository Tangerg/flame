package persistence

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

// notify hands an external commit to the delivery fan-out, which reaches every
// subscribed client. A defect there must cost the notification, not the process:
// the observer is a background loop nobody is waiting on, so an escaping panic
// would take every Session and in-flight Run with it.
func TestExternalChangeObserverSurvivesANotificationDefect(t *testing.T) {
	root := t.TempDir()
	config := Config{DataDirectory: filepath.Join(root, "data")}
	observed, err := Open(t.Context(), config)
	if err != nil {
		t.Fatalf("Open observed Runtime: %v", err)
	}
	t.Cleanup(func() { _ = observed.Close() })
	other, err := Open(t.Context(), config)
	if err != nil {
		t.Fatalf("Open other Runtime: %v", err)
	}
	t.Cleanup(func() { _ = other.Close() })

	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	notifications := make(chan int, 4)
	var mu sync.Mutex
	calls := 0
	done, err := observed.StartExternalChangeObserver(ctx, func() {
		mu.Lock()
		calls++
		call := calls
		mu.Unlock()
		notifications <- call
		if call == 1 {
			panic("persistence: notification fan-out defect")
		}
	})
	if err != nil {
		t.Fatalf("start external change observer: %v", err)
	}

	for index, id := range []string{"session-one", "session-two"} {
		commit := testsupport.MustRestoreSession(session.Snapshot{
			ID: id, Workspace: testsupport.MustWorkspace(root),
		})
		if err := other.Sessions.Insert(t.Context(), commit); err != nil {
			t.Fatalf("insert %q through the other Runtime: %v", id, err)
		}
		select {
		case call := <-notifications:
			if call != index+1 {
				t.Fatalf("notification %d arrived for commit %d", call, index+1)
			}
		case <-time.After(20 * externalChangePollInterval):
			t.Fatalf("commit %d produced no notification; the observer stopped at the defect", index+1)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("observer did not stop with its context after recovering")
	}
}
