package bootstrap

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func TestRecoveryKeepsEachConversationWithItsRunTree(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	ctx := t.Context()
	openedAt := time.Now().UTC()
	type abandonedTree struct {
		sessionID identity.SessionID
		rootID    identity.RunID
		callID    string
	}
	trees := []abandonedTree{
		{testsupport.MustSessionID("ses_recovery_first"), testsupport.MustRunID("run_recovery_first"), "call-first"},
		{testsupport.MustSessionID("ses_recovery_second"), testsupport.MustRunID("run_recovery_second"), "call-second"},
	}
	for _, tree := range trees {
		if err := cfg.stores.Sessions.Insert(ctx, testsupport.MustRestoreSession(session.Snapshot{
			ID: tree.sessionID, Workspace: testsupport.MustWorkspace(cfg.defaultWorkspacePath),
			CreatedAt: openedAt, UpdatedAt: openedAt,
		})); err != nil {
			t.Fatal(err)
		}
		if err := cfg.stores.Runs.Admit(ctx, run.Draft{
			RunID: tree.rootID, SessionID: tree.sessionID,
			SegmentID:      testsupport.MustSegmentID("seg_" + tree.callID),
			ModelSelection: cfg.defaultSelection, CreatedAt: openedAt,
		}); err != nil {
			t.Fatal(err)
		}
		call := chat.ToolCall{ID: tree.callID, Name: "read", Arguments: `{"path":"notes.txt"}`}
		if err := cfg.stores.Conversations.Write(ctx, tree.sessionID,
			chat.NewUserMessage(chat.NewTextPart("Read the notes")),
			chat.NewAssistantMessage(chat.NewToolCallPart(call)),
		); err != nil {
			t.Fatal(err)
		}
	}

	host := assembleTestRuntime(t, cfg)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	const refusal = "refuse the second tree"
	questionCrashDatabase(t, cfg.stores.DataDirectory, `CREATE TRIGGER refuse_second_tree
		BEFORE UPDATE OF outcome ON runs
		WHEN NEW.outcome <> '' AND EXISTS (SELECT 1 FROM runs WHERE outcome <> '')
		BEGIN SELECT RAISE(ABORT, '`+refusal+`'); END`)
	if err := host.application.recoverUncertainExecutions(ctx); err == nil || !strings.Contains(err.Error(), refusal) {
		t.Fatalf("recovery did not reach the second tree: %v", err)
	}

	read := func(tree abandonedTree) (run.Run, []chat.Message) {
		t.Helper()
		stored, err := cfg.stores.Runs.ListRuns(ctx, tree.sessionID)
		if err != nil || len(stored) != 1 || stored[0].ID() != tree.rootID {
			t.Fatalf("read original Run %q: %+v, %v", tree.rootID, stored, err)
		}
		messages, err := cfg.stores.Conversations.Read(ctx, tree.sessionID)
		if err != nil {
			t.Fatal(err)
		}
		return stored[0], messages
	}
	const openMessageCount = 2
	for _, tree := range trees {
		root, messages := read(tree)
		if root.State() != run.Running || len(messages) != openMessageCount {
			t.Fatalf("failed recovery changed Session %q: %s with %d messages", tree.sessionID, root.State(), len(messages))
		}
	}

	questionCrashDatabase(t, cfg.stores.DataDirectory, "DROP TRIGGER refuse_second_tree")
	if err := host.application.recoverUncertainExecutions(ctx); err != nil {
		t.Fatal(err)
	}
	for _, tree := range trees {
		root, messages := read(tree)
		_, lost := root.Loss()
		mark, known := root.MessageMark().Count()
		if !lost || len(messages) != openMessageCount+1 || !known || mark != len(messages) {
			t.Fatalf("recovery left inconsistent Run %q: %s, mark %s, messages %d", root.ID(), root.State(), root.MessageMark(), len(messages))
		}
		closure := messages[openMessageCount]
		if closure.Role != chat.RoleTool || len(closure.Parts) != 1 ||
			closure.Parts[0].ToolResult == nil || closure.Parts[0].ToolResult.ID != tree.callID {
			t.Fatalf("Run %q closed a different ToolCall: %+v", root.ID(), closure)
		}
	}
}
