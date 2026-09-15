package bootstrap

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

// TestUnfinishedFileRollbackFencesTheSessionUntilRecovery walks the sequence a
// unit test cannot: an operation that stopped with its recovery intent logged,
// then the next command, then a query, then a restart, then the next command
// again. The intent is the only record that a reset may have changed part of the
// tree, so until recovery re-drives it the Session admits no Run and no other
// rollback may displace it — and after recovery the Session works again.
func TestUnfinishedFileRollbackFencesTheSessionUntilRecovery(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed; file rollback recovery needs the checkpoint store")
	}
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	if out, err := exec.Command("git", "-C", home, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init workspace: %v: %s", err, out)
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
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "rollback recovery",
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

	// The crash window: the intent committed, the reset may have changed part of
	// the tree, and nothing cleared it.
	unfinished := sessions.WorkspaceMutation{
		SessionID: session.ID, CWD: session.Workspace.Ref.Path,
		ToRunID: started.RunID, RestoreHistory: true,
	}
	if err := stores.WorkspaceMutations.Record(ctx, unfinished); err != nil {
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
	stores, err := persistence.Open(t.Context(), persistence.Config{
		DataDirectory:        dataDirectory,
		DefaultWorkspacePath: dataDirectory,
	})
	if err != nil {
		t.Fatalf("open persistence: %v", err)
	}
	cfg := protocolRuntimeConfig(t, stores, model)
	host, api := buildProtocolRuntime(t, cfg, stores.DataDirectory)
	return host, api, stores
}
