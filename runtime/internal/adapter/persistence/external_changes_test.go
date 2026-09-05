package persistence

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestObserveExternalChangesIgnoresLocalAndReportsOtherRuntimeCommit(t *testing.T) {
	root := t.TempDir()
	config := Config{DataDirectory: filepath.Join(root, "data"), DefaultWorkspacePath: root}
	first, err := Open(t.Context(), config)
	if err != nil {
		t.Fatalf("Open first: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(t.Context(), config)
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	changed := make(chan struct{}, 4)
	done, err := first.StartExternalChangeObserver(ctx, func() { changed <- struct{}{} }, func(err error) { t.Errorf("observer: %v", err) })
	if err != nil {
		t.Fatalf("start external change observer: %v", err)
	}
	local := testsupport.MustRestoreSession(session.Snapshot{ID: "session-local", Workspace: testsupport.MustWorkspace(root)})
	if err := first.Sessions.Insert(t.Context(), local); err != nil {
		t.Fatalf("insert through observed Runtime: %v", err)
	}
	select {
	case <-changed:
		t.Fatal("local commit produced an external-change notification")
	case <-time.After(2 * externalChangePollInterval):
	}

	remote := testsupport.MustRestoreSession(session.Snapshot{ID: "session-remote", Workspace: testsupport.MustWorkspace(root)})
	if err := second.Sessions.Insert(t.Context(), remote); err != nil {
		t.Fatalf("insert through second Runtime: %v", err)
	}
	select {
	case <-changed:
	case <-time.After(10 * externalChangePollInterval):
		t.Fatal("other Runtime commit did not trigger convergence")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("observer did not stop with its context")
	}
}

func TestExternalChangeObserverReportsDatabaseFailure(t *testing.T) {
	bundle, err := Open(t.Context(), Config{DataDirectory: t.TempDir(), DefaultWorkspacePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	failures := make(chan error, 1)
	done, err := bundle.StartExternalChangeObserver(ctx, func() { t.Error("failed read reported a commit") }, func(err error) { failures <- err })
	if err != nil {
		t.Fatal(err)
	}
	if err := bundle.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-failures:
		if err == nil {
			t.Fatal("observer omitted failure cause")
		}
	case <-time.After(time.Second):
		t.Fatal("observer silently ignored database failure")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("observer did not stop after a failure")
	}
}
