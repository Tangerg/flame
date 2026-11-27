package bootstrap

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
)

func TestTaskWorktreesKeepSeparateChangesAndHandoffAfterRestart(t *testing.T) {
	source := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", append([]string{
			"-c", "user.name=Worktree Fixture", "-c", "user.email=fixture@example.test",
			"-c", "core.hooksPath=" + os.DevNull,
		}, args...)...)
		cmd.Dir = source
		raw, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, raw, err)
		}
		return strings.TrimSpace(string(raw))
	}
	write := func(root, name string, body []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	requireContents := func(root, name string, want []byte) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s in %s = %q, %v", name, root, got, err)
		}
	}
	git("init", "--quiet", "--template=", "--initial-branch=target")
	write(source, "shared.txt", []byte("base\n"))
	write(source, ".gitignore", []byte("setup.marker\n"))
	git("add", ".")
	git("commit", "--quiet", "-m", "initial snapshot")
	base := git("rev-parse", "HEAD")

	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	create := func(key, setup string) *protocol.TaskWorktree {
		t.Helper()
		return r.must(delivery.TaskWorktreesCreate, protocol.CreateTaskWorktreeRequest{
			Source: protocol.WorkspaceRef{Path: source}, Base: "target", Setup: setup,
		}, key).(*protocol.TaskWorktree)
	}
	a := create("worktree-create-a", "printf ready > setup.marker")
	b := create("worktree-create-b", "")
	unused := create("worktree-create-unused", "")
	if !a.Available || !b.Available || a.Base != base || b.Base != base || a.Workspace == b.Workspace {
		t.Fatalf("task worktrees did not reserve separate directories at %s: %+v %+v", base, a, b)
	}
	if sandbox.Available() {
		if a.Setup == nil || a.Setup.ExitCode == nil || *a.Setup.ExitCode != 0 {
			t.Fatalf("task setup did not finish in the managed workspace: %+v", a.Setup)
		}
		requireContents(a.Workspace.Path, "setup.marker", []byte("ready"))
	}
	open := func(task *protocol.TaskWorktree, key string) *protocol.Session {
		t.Helper()
		return r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{
			Title: key, Workspace: &task.Workspace,
		}, key).(*protocol.Session)
	}
	first, second := open(a, "worktree-session-a"), open(b, "worktree-session-b")
	write(a.Workspace.Path, "shared.txt", []byte("first task\n"))
	write(b.Workspace.Path, "shared.txt", []byte("second task\n"))
	binary := []byte{0, 15, 255, 89}
	write(b.Workspace.Path, "attachment.bin", binary)
	requireContents(source, "shared.txt", []byte("base\n"))

	r.close()
	if err := cfg.stores.Close(); err != nil {
		t.Fatal(err)
	}
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: cfg.stores.DataDirectory, ValidateDirectory: persistencefixture.UnleasedDirectory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	cfg.stores = stores
	cfg.providerRegistry = stores.Providers
	r = poMCPOpen(t, cfg)
	for _, want := range []*protocol.Session{first, second} {
		found := r.must(delivery.SessionsGet, protocol.GetSessionRequest{SessionID: want.ID}, "").(*protocol.Session)
		if found.Workspace != want.Workspace {
			t.Fatalf("restart changed the session workspace: %+v", found.Workspace)
		}
	}
	list := r.must(delivery.TaskWorktreesList, struct{}{}, "").(*protocol.Page[protocol.TaskWorktree])
	if len(list.Data) != 3 {
		t.Fatalf("restart lost worktree custody: %+v", list.Data)
	}
	preview := func(task *protocol.TaskWorktree) *protocol.WorktreeHandoff {
		t.Helper()
		return r.must(delivery.TaskWorktreesPreview, protocol.TaskWorktreeRequest{ID: task.ID}, "").(*protocol.WorktreeHandoff)
	}
	if patch := preview(a).Patch; !strings.Contains(patch, "+first task") || strings.Contains(patch, "+second task") {
		t.Fatalf("the first task preview mixed worktrees: %q", patch)
	}
	chosen := preview(b)
	if chosen.Branch != "target" || chosen.Head != base || !strings.Contains(chosen.Patch, "+second task") || !strings.Contains(chosen.Patch, "attachment.bin") {
		t.Fatalf("the handoff preview lacks its exact destination and changes: %+v", chosen)
	}
	write(b.Workspace.Path, "shared.txt", []byte("second task updated\n"))
	stale := r.endpoint.Invoke(t.Context(), delivery.TaskWorktreesApply, protocol.ApplyWorktreeHandoffRequest{ID: b.ID, Digest: chosen.Digest},
		protocol.CallMetadata{IdempotencyKey: "stale-worktree-handoff"})
	if !errors.Is(stale.Failure, protocol.ErrRevisionConflict) {
		t.Fatalf("stale worktree handoff was not fenced: %v", stale.Failure)
	}
	requireContents(source, "shared.txt", []byte("base\n"))
	chosen = preview(b)
	r.must(delivery.TaskWorktreesApply, protocol.ApplyWorktreeHandoffRequest{ID: b.ID, Digest: chosen.Digest}, "apply-worktree-handoff")
	r.must(delivery.TaskWorktreesApply, protocol.ApplyWorktreeHandoffRequest{ID: b.ID, Digest: chosen.Digest}, "apply-worktree-handoff")
	requireContents(source, "shared.txt", []byte("second task updated\n"))
	requireContents(source, "attachment.bin", binary)
	requireContents(a.Workspace.Path, "shared.txt", []byte("first task\n"))
	requireContents(b.Workspace.Path, "shared.txt", []byte("second task updated\n"))

	busy := r.endpoint.Invoke(t.Context(), delivery.TaskWorktreesRelease, protocol.TaskWorktreeRequest{ID: a.ID},
		protocol.CallMetadata{IdempotencyKey: "release-worktree-in-use"})
	if !errors.Is(busy.Failure, protocol.ErrSessionBusy) {
		t.Fatalf("released a worktree with a bound session: %v", busy.Failure)
	}
	r.must(delivery.TaskWorktreesRelease, protocol.TaskWorktreeRequest{ID: unused.ID}, "release-unused-worktree")
	if _, err := os.Stat(unused.Workspace.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("released workspace still exists: %v", err)
	}
	requireContents(source, "shared.txt", []byte("second task updated\n"))
}
