package bootstrap

import (
	"context"
	"errors"
	"iter"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestTaskWorktreeCatalogReportsOnlyCommittedChanges(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	git("commit", "--quiet", "--allow-empty", "-m", "initial checkout")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	ctx, cancel := context.WithTimeout(protocolLifecycleContext(t.Context()), lifecycleWaitBudget(t))
	defer cancel()
	_, changes, err := r.api.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicTaskWorktreesChanged, protocol.TopicSessionsChanged},
	})
	if err != nil {
		t.Fatal(err)
	}
	next, stop := iter.Pull2(changes)
	defer stop()
	expect := func(want protocol.RuntimeEventType) protocol.RuntimeEvent {
		t.Helper()
		event, err, ok := next()
		if !ok || err != nil || event.Type != want {
			t.Fatalf("next notification = %+v, %v, observed=%t; want %s", event, err, ok, want)
		}
		return event
	}
	expectSessionBarrier := func(key string) {
		t.Helper()
		session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: key}, key).(*protocol.Session)
		event := expect(protocol.RuntimeSessionsChanged)
		if len(event.SessionIDs) != 1 || event.SessionIDs[0] != session.ID {
			t.Fatalf("session notification = %+v; want %s", event, session.ID)
		}
	}

	task := r.must(delivery.TaskWorktreesCreate, protocol.CreateTaskWorktreeRequest{
		Source: protocol.WorkspaceRef{Path: source}, Base: "HEAD",
	}, "create-observed-worktree").(*protocol.TaskWorktree)
	expect(protocol.RuntimeTaskWorktreesChanged)
	draft := filepath.Join(task.Workspace.Path, "draft.txt")
	if err := os.WriteFile(draft, []byte("unfinished work\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := protocol.TaskWorktreeRequest{ID: task.ID}
	refusal := r.endpoint.Invoke(ctx, delivery.TaskWorktreesRelease, request, protocol.CallMetadata{
		IdempotencyKey: "release-dirty-observed-worktree",
	})
	if refusal.Failure == nil || !errors.Is(refusal.Failure, protocol.ErrRevisionConflict) {
		t.Fatalf("dirty release = %+v, %v; want revision conflict", refusal.Value, refusal.Failure)
	}
	expectSessionBarrier("after-refused-worktree-release")

	if err := os.Remove(draft); err != nil {
		t.Fatal(err)
	}
	const key = "release-clean-observed-worktree"
	r.must(delivery.TaskWorktreesRelease, request, key)
	expect(protocol.RuntimeTaskWorktreesChanged)
	r.must(delivery.TaskWorktreesRelease, request, key)
	expectSessionBarrier("after-replayed-worktree-release")
}
