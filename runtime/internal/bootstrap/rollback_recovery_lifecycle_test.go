package bootstrap

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/scope/core/chat"
)

func TestFileRollbackRetriesTheOriginalIntent(t *testing.T) {
	const checkpoint = "checkpoint"
	const originalDraft = "third turn belongs to the original receipt"
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	workspace := t.TempDir()
	if out, err := exec.CommandContext(t.Context(), "git", "-C", workspace, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("initialize disposable workspace: %v: %s", err, out)
	}
	material := filepath.Join(workspace, "material.txt")
	if err := os.WriteFile(material, []byte(checkpoint), 0o600); err != nil {
		t.Fatal(err)
	}
	host, api, stores := openProtocolRuntimeWithStores(t, newReplyStub("ok"))
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: workspace}, Title: "file rollback receipt",
	})
	if err != nil {
		t.Fatal(err)
	}
	var runIDs []string
	for _, input := range []string{"first turn", "second turn", originalDraft} {
		started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
			SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: input}},
		})
		if err != nil {
			t.Fatal(err)
		}
		waitForRunEvents(t, collectRunEvents(events), input)
		waitForProtocolRunTerminal(t, ctx, api, started.RunID)
		runIDs = append(runIDs, started.RunID)
	}
	if err := os.WriteFile(material, []byte("workspace edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	endpoint := host.Endpoint()
	request := protocol.RollbackSessionRequest{SessionID: session.ID, ToRunID: runIDs[0], RestoreType: protocol.RestoreBoth}
	exclude := filepath.Join(workspace, ".git", "info", "exclude")
	if err := os.WriteFile(exclude, []byte("material.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	refusal := protocol.CallMetadata{IdempotencyKey: "file-rollback-checkpoint-conflict"}
	if _, err := endpoint.Call[protocol.RollbackSessionRequest, *protocol.RollbackSessionResponse](ctx, rollbackCrashMethod, request, refusal); !errors.Is(err, protocol.ErrCheckpointConflict) {
		t.Fatalf("restore over ignored, unarchived material = %v, want checkpoint_conflict", err)
	}
	if pending, err := stores.WorkspaceMutations.ListPending(ctx); err != nil || len(pending) != 0 {
		t.Fatalf("definite checkout refusal left a pending intent: %+v, %v", pending, err)
	}
	if err := os.WriteFile(exclude, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := endpoint.Call[protocol.RollbackSessionRequest, *protocol.RollbackSessionResponse](ctx, rollbackCrashMethod, request, refusal); !errors.Is(err, protocol.ErrCheckpointConflict) {
		t.Fatalf("the original refusal did not replay after its cause cleared: %v", err)
	}
	if got, err := os.ReadFile(material); err != nil || string(got) != "workspace edit" {
		t.Fatalf("a replayed checkout refusal moved the working tree: %q, %v", got, err)
	}
	questionCrashDatabase(t, home, `CREATE TRIGGER file_rollback_history_refusal BEFORE DELETE ON runs
		BEGIN SELECT RAISE(ABORT, 'test refused the rollback commit'); END`)
	meta := protocol.CallMetadata{IdempotencyKey: "original-file-rollback"}
	if value, err := endpoint.Call[protocol.RollbackSessionRequest, *protocol.RollbackSessionResponse](ctx, rollbackCrashMethod, request, meta); value != nil || !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("refused history commit = %+v, %v, want a retained original outcome", value, err)
	}
	questionCrashDatabase(t, home, "DROP TRIGGER file_rollback_history_refusal")
	if got, err := os.ReadFile(material); err != nil || string(got) != checkpoint {
		t.Fatalf("file restore did not reach checkout before the history refusal: %q, %v", got, err)
	}
	if _, err := api.RollbackSession(ctx, protocol.RollbackSessionRequest{
		SessionID: session.ID, ToRunID: runIDs[1], RestoreType: protocol.RestoreHistory,
	}); !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("history-only rollback could displace the original draft: %v", err)
	}
	if _, err := endpoint.Call[protocol.RollbackSessionRequest, *protocol.RollbackSessionResponse](ctx, rollbackCrashMethod, request,
		protocol.CallMetadata{IdempotencyKey: "different-file-rollback"}); !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("another key could adopt the original rollback: %v", err)
	}
	changed := request
	changed.RestoreType = protocol.RestoreFiles
	if _, err := endpoint.Call[protocol.RollbackSessionRequest, *protocol.RollbackSessionResponse](ctx, rollbackCrashMethod, changed, meta); !errors.Is(err, protocol.ErrIdempotencyConflict) {
		t.Fatalf("original key admitted changed restore scope: %v", err)
	}
	if history, err := api.ListRuns(ctx, protocol.ListRunsRequest{SessionID: session.ID}); err != nil || len(history.Data) != 3 {
		t.Fatalf("unfinished rollback lost its inputs: %+v, %v", history, err)
	}
	result, err := endpoint.Call[protocol.RollbackSessionRequest, *protocol.RollbackSessionResponse](ctx, rollbackCrashMethod, request, meta)
	if err != nil || result == nil || len(result.DroppedRuns) != 2 {
		t.Fatalf("original retry did not complete both parts: %+v, %v", result, err)
	}
	third := result.DroppedRuns[1]
	if third.Run.ID != runIDs[2] || len(third.UserInput) != 1 || third.UserInput[0].Text != originalDraft {
		t.Fatalf("original retry lost the third Run's draft: %+v", third)
	}
	if pending, err := stores.WorkspaceMutations.ListPending(ctx); err != nil || len(pending) != 0 {
		t.Fatalf("completed rollback left a pending intent: %+v, %v", pending, err)
	}
}

// TestUnfinishedFileRollbackFencesTheSessionUntilRecovery walks the sequence a
// unit test cannot: an operation that stopped with its recovery intent logged,
// then the next command, then a query, then a restart, then the next command
// again. The intent is the only record that a checkout may have changed part of the
// tree, so until recovery re-drives it the Session admits no Run and no other
// rollback may displace it — and after recovery the Session works again.
func TestUnfinishedFileRollbackFencesTheSessionUntilRecovery(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; file rollback recovery needs the checkpoint store")
	}
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	workspace := t.TempDir()
	if out, err := exec.Command("git", "-C", workspace, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init workspace: %v: %s", err, out)
	}
	material := filepath.Join(workspace, "material.txt")
	if err := os.WriteFile(material, []byte("checkpoint material"), 0o644); err != nil {
		t.Fatal(err)
	}
	model := newReplyStub("ok")
	ctx := protocolLifecycleContext(t.Context())

	first, api, stores := openProtocolRuntimeWithStores(t, model)
	closed := false
	t.Cleanup(func() {
		if !closed {
			if err := first.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: workspace}, Title: "rollback recovery",
	})
	if err != nil {
		t.Fatalf("sessions.create: %v", err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "first turn"}},
	})
	if err != nil {
		t.Fatalf("runs.start: %v", err)
	}
	waitForRunEvents(t, collectRunEvents(events), "first turn")
	waitForProtocolRunTerminal(t, ctx, api, started.RunID)
	if err := os.WriteFile(material, []byte("material after the checkpoint"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The crash window: the intent committed, the checkout may have changed part of
	// the tree, and nothing cleared it.
	unfinished := sessions.WorkspaceMutation{
		WorkspaceRollbackSpec: sessions.WorkspaceRollbackSpec{
			SessionID: session.ID, ToRunID: started.RunID, RestoreHistory: true,
		},
		CWD: session.Workspace.Ref.Path,
	}
	if _, err := stores.WorkspaceMutations.Record(ctx, unfinished); err != nil {
		t.Fatalf("record unfinished rollback: %v", err)
	}

	if _, _, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "second turn"}},
	}); !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("runs.start over an unrecovered tree = %v, want session_busy", err)
	}
	if _, err := api.RollbackSession(ctx, protocol.RollbackSessionRequest{
		SessionID: session.ID, ToRunID: started.RunID, RestoreType: protocol.RestoreFiles,
	}); !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("a second rollback over an unrecovered tree = %v, want session_busy", err)
	}
	if pending, err := stores.WorkspaceMutations.ListPending(ctx); err != nil ||
		len(pending) != 1 || pending[0] != unfinished {
		t.Fatalf("pending = (%+v, %v), want the unfinished operation intact", pending, err)
	}

	// The restart is what owns re-driving it.
	closed = true
	if err := first.Close(); err != nil {
		t.Fatalf("close Runtime: %v", err)
	}
	restarted, api, stores := openProtocolRuntimeWithStores(t, model)
	t.Cleanup(func() {
		if err := restarted.Close(); err != nil {
			t.Error(err)
		}
	})
	if pending, err := stores.WorkspaceMutations.ListPending(ctx); err != nil || len(pending) != 0 {
		t.Fatalf("pending after recovery = (%+v, %v), want none", pending, err)
	}
	if content, err := os.ReadFile(material); err != nil || string(content) != "checkpoint material" {
		t.Fatalf("material after recovery = %q, error = %v", content, err)
	}
	resumedRun, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "after recovery"}},
	})
	if err != nil {
		t.Fatalf("runs.start after recovery: %v", err)
	}
	waitForRunEvents(t, collectRunEvents(events), "turn after recovery")
	waitForProtocolRunTerminal(t, ctx, api, resumedRun.RunID)
}

// openProtocolRuntimeWithStores opens a Runtime over FLAME_HOME and hands back
// the same durable stores it runs on, so a test can write the state a crash
// would have left without a second connection racing the live one.
func openProtocolRuntimeWithStores(
	t *testing.T,
	model chat.Model,
) (*Instance, *delivery.Handler, *persistence.Bundle) {
	t.Helper()
	dataDirectory := os.Getenv("FLAME_HOME")
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: dataDirectory})
	if err != nil {
		t.Fatalf("open persistence: %v", err)
	}
	cfg := protocolRuntimeConfig(t, stores, model)
	host, api := buildProtocolRuntime(t, cfg, stores.DataDirectory)
	return host, api, stores
}
