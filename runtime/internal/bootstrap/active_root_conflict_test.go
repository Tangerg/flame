package bootstrap

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestRunOpeningConflictReportsTheActiveRoot(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	instance := poMCPOpen(t, cfg)
	ctx := t.Context()
	session, err := instance.api.CreateSession(ctx, protocol.CreateSessionRequest{Title: "Run conflict"})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := testsupport.MustSessionID(session.ID)
	rootID := testsupport.MustRunID("run_z_root")
	childID := testsupport.MustRunID("run_a_child")
	itemID := testsupport.MustItemID("item_spawned_delegate")
	openedAt := time.Now().UTC()
	if err := cfg.stores.Runs.Admit(ctx, run.Draft{
		RunID: rootID, SessionID: sessionID,
		SegmentID: testsupport.MustSegmentID("seg_root"), CreatedAt: openedAt, ModelSelection: cfg.defaultSelection,
	}); err != nil {
		t.Fatal(err)
	}
	source, err := tool.BuiltIn(builtin.DelegateTask)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.stores.Transcript.AppendItem(ctx, testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: itemID, SessionID: sessionID, RunID: rootID, OccurredAt: openedAt,
		Kind: transcript.ToolCall, Status: transcript.ItemRunning,
		Tool: &transcript.ToolInvocation{Source: source, Name: string(builtin.DelegateTask)},
	})); err != nil {
		t.Fatal(err)
	}
	if err := cfg.stores.Runs.Admit(ctx, run.Draft{
		RunID: childID, SessionID: sessionID, SpawnedByItemID: itemID, ParentRunID: rootID, RootRunID: rootID,
		SegmentID: testsupport.MustSegmentID("seg_child"), CreatedAt: openedAt, ModelSelection: cfg.defaultSelection,
	}); err != nil {
		t.Fatal(err)
	}

	result := instance.endpoint.Invoke(ctx, delivery.RunsStart, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Start new root work"}},
	}, protocol.CallMetadata{IdempotencyKey: "competing-root-start"})
	if !errors.Is(result.Failure, protocol.ErrSessionHasActiveRun) {
		t.Fatalf("opening did not report a conflicting Run: %+v", result)
	}
	active := result.Failure.Problem().ActiveRun
	if active == nil || active.RunID != rootID.String() || active.Status != protocol.RunStatusRunning {
		t.Fatalf("opening conflict points to %+v, want the original root %s", active, rootID)
	}
}
