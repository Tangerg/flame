package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestTaskWorktreeHandoffTargetsTheOriginalCheckoutHead(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	git("commit", "--quiet", "--allow-empty", "-m", "initial checkout")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	parent := r.must(delivery.TaskWorktreesCreate, protocol.CreateTaskWorktreeRequest{
		Source: protocol.WorkspaceRef{Path: source}, Base: "target",
	}, "detached-handoff-parent").(*protocol.TaskWorktree)
	child := r.must(delivery.TaskWorktreesCreate, protocol.CreateTaskWorktreeRequest{
		Source: parent.Workspace, Base: "HEAD",
	}, "detached-handoff-child").(*protocol.TaskWorktree)
	const file, contents = "handed-off.txt", "handed off in the selected checkout"
	if err := os.WriteFile(filepath.Join(child.Workspace.Path, file), []byte(contents+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := protocol.TaskWorktreeRequest{ID: child.ID}
	detached := r.must(delivery.TaskWorktreesPreview, request, "").(*protocol.WorktreeHandoff)
	if detached.Head != parent.Base || detached.Branch != nil || detached.Worktree.Source != parent.Workspace {
		t.Fatalf("detached handoff targets %+v; want checkout %s at %s", detached, parent.Workspace.Path, parent.Base)
	}
	apply := protocol.ApplyWorktreeHandoffRequest{ID: child.ID, Digest: detached.Digest}
	const branch = "handoff-target"
	git("-C", parent.Workspace.Path, "switch", "--quiet", "-c", branch)
	attached := r.must(delivery.TaskWorktreesPreview, request, "").(*protocol.WorktreeHandoff)
	if attached.Head != detached.Head || attached.Branch == nil || *attached.Branch != branch || attached.Digest == detached.Digest {
		t.Fatalf("branch change at the same commit did not change the handoff: %+v; old = %+v", attached, detached)
	}
	refused := r.endpoint.Invoke(protocolLifecycleContext(t.Context()), delivery.TaskWorktreesApply, apply,
		protocol.CallMetadata{IdempotencyKey: "detached-handoff-stale"})
	if refused.Failure == nil || !errors.Is(refused.Failure, protocol.ErrRevisionConflict) {
		t.Fatalf("apply after switching branches = %+v, %v", refused.Value, refused.Failure)
	}
	if changed := git("-C", parent.Workspace.Path, "status", "--porcelain"); changed != "" {
		t.Fatalf("refused handoff changed the original checkout: %s", changed)
	}
	git("-C", parent.Workspace.Path, "switch", "--quiet", "--detach")
	r.must(delivery.TaskWorktreesApply, apply, "apply-detached-handoff")
	if got := git("-C", parent.Workspace.Path, "show", ":"+file); got != contents {
		t.Fatalf("staged checkout contents = %q; want %q", got, contents)
	}
	if _, err := os.Stat(filepath.Join(source, file)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("handoff changed the primary checkout: %v", err)
	}
}
