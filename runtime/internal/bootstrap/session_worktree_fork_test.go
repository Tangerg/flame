package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestTaskWorktreesForkPreviewBindsHistoryHeadAndUncommittedFiles(t *testing.T) {
	source, git := taskWorktreeTestRepository(t)
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(source, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("tracked.txt", "base\n")
	git("add", ".")
	git("commit", "--quiet", "-m", "initial snapshot")
	cfg := poMCPConfig(t)
	cfg.defaultWorkspacePath = source
	r := poMCPOpen(t, cfg)
	parent := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: source}, Title: "original history",
	}, "worktree-fork-source").(*protocol.Session)
	_, events, err := r.api.StartRun(t.Context(), protocol.StartRunRequest{
		SessionID: parent.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Keep this completed history"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "source session history")
	write("tracked.txt", "after\n")
	write("a-untracked.txt", "carried addition\n")
	preview := func() *protocol.SessionForkPreview {
		t.Helper()
		return r.must(delivery.Name("sessions.fork.preview"), protocol.PreviewSessionForkRequest{SessionID: parent.ID}, "").(*protocol.SessionForkPreview)
	}
	fork := func(confirmation protocol.ForkWorktreeConfirmation, key string) protocol.OperationResult {
		t.Helper()
		return r.endpoint.Invoke(t.Context(), delivery.Name("sessions.fork"), protocol.ForkSessionRequest{
			SessionID: parent.ID, Worktree: &confirmation,
		}, protocol.CallMetadata{RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)}, IdempotencyKey: key})
	}
	stale := preview()
	if stale.Runs == 0 || stale.Messages == 0 || stale.FromRunID == "" || stale.Source.Path != parent.Workspace.Ref.Path ||
		stale.Destination.Path == source || stale.Head != git("rev-parse", "HEAD") || !strings.Contains(stale.Patch, "carried addition") {
		t.Fatalf("preview does not describe the original history and local changes: %+v", stale)
	}
	if _, err := os.Stat(stale.Destination.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preview created its proposed worktree: %v", err)
	}
	write("tracked.txt", "edited after preview\n")
	if result := fork(stale.Worktree, "refuse-stale-worktree-fork"); !errors.Is(result.Failure, protocol.ErrRevisionConflict) {
		t.Fatalf("stale worktree fork was not refused: %+v", result)
	}
	current := preview()
	const key = "confirm-the-original-worktree-fork"
	result := fork(current.Worktree, key)
	if result.Failure != nil {
		t.Fatal(result.Failure)
	}
	child := result.Value.(*protocol.Session)
	if child.ID == parent.ID || child.Workspace.Ref.Path != current.Destination.Path {
		t.Fatalf("the fork does not own the previewed worktree: %+v", child)
	}
	for file, want := range map[string]string{"tracked.txt": "edited after preview\n", "a-untracked.txt": "carried addition\n"} {
		content, err := os.ReadFile(filepath.Join(child.Workspace.Ref.Path, file))
		if err != nil || string(content) != want {
			t.Fatalf("fork copy of %s = %q, %v", file, content, err)
		}
	}
	if items, err := r.api.GetSessionSnapshot(t.Context(), protocol.GetSessionSnapshotRequest{SessionID: child.ID}); err != nil || len(items.Runs) == 0 {
		t.Fatalf("fork lost its retained history: %+v, %v", items, err)
	}
	r = reopenPluginRuntime(t, r, &cfg)
	replay := fork(current.Worktree, key)
	if replay.Failure != nil || replay.Value.(*protocol.Session).ID != child.ID {
		t.Fatalf("repeat confirmation replaced its Session: %+v", replay)
	}
	if again := fork(current.Worktree, "a-second-confirmation-is-not-a-new-fork"); !errors.Is(again.Failure, protocol.ErrRevisionConflict) {
		t.Fatalf("another command created a second Session for the preview: %+v", again)
	}
	worktrees := r.must(delivery.TaskWorktreesList, struct{}{}, "").(*protocol.Page[protocol.TaskWorktree])
	if len(worktrees.Data) != 1 || worktrees.Data[0].Workspace.Path != child.Workspace.Ref.Path || !worktrees.Data[0].Available {
		t.Fatalf("worktree receipt replaced or leaked a checkout: %+v", worktrees)
	}
	if git("status", "--porcelain") != "M tracked.txt\n?? a-untracked.txt" {
		t.Fatal("fork altered the source repository index")
	}
}
