package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestTaskWorktreeReleaseFencesNewSessionBinding(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Git timing fixture uses a POSIX shell")
	}
	source, git := taskWorktreeTestRepository(t)
	git("commit", "--quiet", "--allow-empty", "-m", "initial checkout")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	task := r.must(delivery.TaskWorktreesCreate, protocol.CreateTaskWorktreeRequest{
		Source: protocol.WorkspaceRef{Path: source}, Base: "target",
	}, "release-binding-task").(*protocol.TaskWorktree)
	original := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Workspace: &task.Source},
		"session-outside-task").(*protocol.Session)
	archive, err := r.api.ExportSession(t.Context(), protocol.ExportSessionRequest{SessionID: original.ID})
	if err != nil {
		t.Fatal(err)
	}

	alias := filepath.Join(t.TempDir(), "task")
	if err := os.Symlink(task.Workspace.Path, alias); err != nil {
		t.Fatal(err)
	}
	archive.Artifact.Session.Workspace = task.Workspace
	entered, proceed := holdTaskGitBoundary(t, beforeCheckoutRemoval)

	ctx, cancel := context.WithTimeout(protocolLifecycleContext(t.Context()), lifecycleWaitBudget(t))
	release := make(chan protocol.OperationResult, 1)
	var requests sync.WaitGroup
	defer func() {
		cancel()
		requests.Wait()
	}()
	requests.Go(func() {
		release <- r.endpoint.Invoke(ctx, delivery.TaskWorktreesRelease, protocol.TaskWorktreeRequest{ID: task.ID},
			protocol.CallMetadata{IdempotencyKey: "release-before-session-binding"})
	})
	awaitTaskGitBoundary(t, ctx, entered, release)

	create := protocol.CreateSessionRequest{Workspace: &task.Workspace}
	const createKey = "bind-during-release"
	opening := protocol.StartRunRequest{
		NewSession: &protocol.RunSessionOpening{
			SessionID: "ses_release_binding", RunID: "run_release_binding", Workspace: &task.Workspace,
		},
		Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Begin in the task checkout"}},
	}
	for _, binding := range []struct {
		method  delivery.Name
		request any
		key     string
		refusal error
	}{
		{delivery.SessionsCreate, create, createKey, protocol.ErrWorkspaceUnavailable},
		{delivery.SessionsUpdate, protocol.UpdateSessionRequest{
			SessionID: original.ID, ExpectedRevision: original.Revision, Workspace: &protocol.WorkspaceRef{Path: alias},
		}, "relocate-during-release", protocol.ErrWorkspaceUnavailable},
		{delivery.SessionsImport, protocol.ImportSessionRequest{Artifact: *archive.Artifact}, "import-during-release", protocol.ErrWorkspaceUnavailable},
		{delivery.RunsStart, opening, "open-during-release", protocol.ErrSessionBusy},
	} {
		result := r.endpoint.Invoke(ctx, binding.method, binding.request, protocol.CallMetadata{IdempotencyKey: binding.key})
		if result.Failure == nil || !errors.Is(result.Failure, binding.refusal) {
			t.Errorf("%s during Git removal = %+v, %v; want %v", binding.method, result.Value, result.Failure, binding.refusal)
		}
	}
	r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Workspace: &task.Source}, "bind-independent-source")
	if err := os.WriteFile(proceed, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-release:
		if result.Failure != nil {
			t.Fatalf("release did not finish: %v", result.Failure)
		}
	case <-ctx.Done():
		t.Fatalf("waiting for Git worktree removal: %v", ctx.Err())
	}
	if _, err := os.Stat(task.Workspace.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("release did not remove the checkout: %v", err)
	}
	surviving, err := r.api.GetSession(ctx, original.ID)
	if err != nil || surviving.Workspace.Ref != original.Workspace.Ref || surviving.Revision != original.Revision {
		t.Fatalf("source Session after refused relocation and import = %+v, %v; want %+v", surviving, err, original)
	}
	replayed := r.endpoint.Invoke(ctx, delivery.SessionsCreate, create, protocol.CallMetadata{IdempotencyKey: createKey})
	if replayed.Failure == nil || !errors.Is(replayed.Failure, protocol.ErrWorkspaceUnavailable) || !replayed.Failure.RPCData().RecordedOutcome {
		t.Fatalf("recorded binding refusal after checkout removal = %+v, %v", replayed.Value, replayed.Failure)
	}
	fresh := r.endpoint.Invoke(ctx, delivery.RunsStart, opening, protocol.CallMetadata{IdempotencyKey: "open-after-release"})
	if fresh.Failure == nil || !errors.Is(fresh.Failure, protocol.ErrWorkspaceUnavailable) {
		t.Fatalf("new Run opening after checkout removal = %+v, %v", fresh.Value, fresh.Failure)
	}
}

func TestTaskWorktreeForkCannotInheritReleasedWorkspace(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	git("commit", "--quiet", "--allow-empty", "-m", "initial checkout")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	type pauseForkSnapshot struct{}
	captured := make(chan struct{})
	resume := make(chan struct{})
	read := cfg.stores.ReadSnapshot
	cfg.stores.ReadSnapshot = func(ctx context.Context, load func(context.Context) error) error {
		err := read(ctx, load)
		if err != nil || ctx.Value(pauseForkSnapshot{}) == nil {
			return err
		}
		close(captured)
		select {
		case <-resume:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	r := poMCPOpen(t, cfg)
	task := r.must(delivery.TaskWorktreesCreate, protocol.CreateTaskWorktreeRequest{
		Source: protocol.WorkspaceRef{Path: source}, Base: "target",
	}, "fork-release-task").(*protocol.TaskWorktree)
	parent := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Workspace: &task.Workspace},
		"fork-release-parent").(*protocol.Session)
	ctx, cancel := context.WithTimeout(protocolLifecycleContext(t.Context()), lifecycleWaitBudget(t))
	forked := make(chan protocol.OperationResult, 1)
	var request sync.WaitGroup
	defer func() {
		cancel()
		request.Wait()
	}()
	const forkMethod delivery.Name = "sessions.fork"
	input := protocol.ForkSessionRequest{SessionID: parent.ID}
	request.Go(func() {
		paused := context.WithValue(ctx, pauseForkSnapshot{}, true)
		forked <- r.endpoint.Invoke(paused, forkMethod, input, protocol.CallMetadata{IdempotencyKey: "fork-before-relocation"})
	})
	select {
	case <-captured:
	case result := <-forked:
		t.Fatalf("fork settled before reading its parent snapshot: %+v, %v", result.Value, result.Failure)
	case <-ctx.Done():
		t.Fatalf("fork did not reach its parent snapshot: %v", ctx.Err())
	}
	r.must(delivery.SessionsUpdate, protocol.UpdateSessionRequest{
		SessionID: parent.ID, ExpectedRevision: parent.Revision, Workspace: &task.Source,
	}, "relocate-fork-parent")
	r.must(delivery.TaskWorktreesRelease, protocol.TaskWorktreeRequest{ID: task.ID}, "release-fork-workspace")
	close(resume)
	select {
	case result := <-forked:
		if result.Failure == nil || !errors.Is(result.Failure, protocol.ErrRevisionConflict) {
			t.Fatalf("fork from the old workspace = %+v, %v; want revision conflict", result.Value, result.Failure)
		}
	case <-ctx.Done():
		t.Fatalf("fork did not verify the committed parent workspace: %v", ctx.Err())
	}
	child := r.must(forkMethod, input, "fork-after-relocation").(*protocol.Session)
	if child.Workspace.Ref != task.Source {
		t.Fatalf("fork after relocation bound to %+v; want %+v", child.Workspace.Ref, task.Source)
	}
}
