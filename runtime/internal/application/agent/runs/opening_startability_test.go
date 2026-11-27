package runs

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/application/ownership"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

func TestWaitSessionStartableFollowsWorkspaceEdit(t *testing.T) {
	oldWorkspace, err := session.NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	newWorkspace, err := session.NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id, err := identity.ParseSession("ses_waiting_for_workspace")
	if err != nil {
		t.Fatal(err)
	}
	selection, err := flamemodel.New("anthropic", "claude-test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	initial, err := session.New(session.Draft{ID: id, Workspace: oldWorkspace, Selection: selection, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	gate, err := ownership.NewGate(ownership.GateConfig{
		ResolveTree:              filesystem.CanonicalPath,
		TreesOverlap:             filesystem.PathsOverlap,
		SettleRetirement:         func(context.Context, identity.SessionID) error { return nil },
		WorkspaceRollbackPending: func(context.Context, identity.SessionID) (bool, error) { return false, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	releaseOldTree, acquired, err := gate.AcquireWorkingTreeMutation(oldWorkspace.Path())
	if err != nil || !acquired {
		t.Fatalf("reserve old working tree: acquired=%t, %v", acquired, err)
	}
	defer releaseOldTree()

	reader := &startabilitySession{read: make(chan struct{})}
	reader.current.Store(&initial)
	openings := Openings{sessionReader: reader, activeRoots: reader, admission: gate}
	ctx, stop := context.WithTimeout(t.Context(), 3*time.Second)
	defer stop()
	result := make(chan error, 1)
	go func() { result <- openings.WaitSessionStartable(ctx, id) }()
	select {
	case <-reader.read:
	case <-ctx.Done():
		t.Fatal("wait did not read the old Session")
	}

	release, acquired, err := gate.AcquireSession(ctx, id)
	if err != nil || !acquired {
		t.Fatalf("admit Session edit: acquired=%t, %v", acquired, err)
	}
	updated, changed, err := initial.Edit(initial.Revision(), session.Patch{Workspace: &newWorkspace}, now.Add(time.Second))
	if err != nil || !changed {
		release()
		t.Fatalf("move Session workspace: changed=%t, %v", changed, err)
	}
	reader.current.Store(&updated)
	release()

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("wait remained blocked on the old working tree: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("workspace edit did not wake the wait: %v", ctx.Err())
	}
}

type startabilitySession struct {
	current atomic.Pointer[session.Session]
	read    chan struct{}
	once    sync.Once
}

func (s *startabilitySession) Get(context.Context, identity.SessionID) (session.Session, error) {
	current := *s.current.Load()
	s.once.Do(func() { close(s.read) })
	return current, nil
}

func (s *startabilitySession) ActiveRoot(context.Context, identity.SessionID) (*ActiveRoot, error) {
	return nil, nil
}
