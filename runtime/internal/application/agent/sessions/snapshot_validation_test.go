package sessions

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestSnapshotNormalizeForRestoreProjectsPreviewWithoutMutatingSource(t *testing.T) {
	snapshot := offloadedSnapshot("full body")

	normalized, err := snapshot.NormalizeForRestore()
	if err != nil {
		t.Fatalf("NormalizeForRestore: %v", err)
	}
	normalizedTool, _ := normalized.Items[0].ToolInvocation()
	if got, _ := normalizedTool.Result.String(); got != "bounded preview" {
		t.Fatalf("normalized result = %q, want bounded preview", got)
	}
	sourceTool, _ := snapshot.Items[0].ToolInvocation()
	if got, _ := sourceTool.Result.String(); got != "full body" {
		t.Fatalf("source result mutated to %q", got)
	}
}

func TestSnapshotPortableSnapshotOwnsMessages(t *testing.T) {
	snapshot := portableSnapshotWithMessage()

	portable, err := snapshot.PortableSnapshot()
	if err != nil {
		t.Fatalf("PortableSnapshot: %v", err)
	}
	snapshot.Messages[0].Parts[0].Text = "source mutation"
	if text := portable.Messages[0].Text(); text != "original" {
		t.Fatalf("portable message after source mutation = %q, want owned snapshot", text)
	}
	portable.Messages[0].Parts[0].Text = "portable mutation"
	if text := snapshot.Messages[0].Text(); text != "source mutation" {
		t.Fatalf("source message after portable mutation = %q, want unchanged", text)
	}
}

func TestSnapshotPortableSnapshotOwnsCollections(t *testing.T) {
	snapshot := portableSnapshotWithMessage()
	at := time.Unix(2, 0).UTC()
	result := tool.StringResult("full body")
	snapshot.Items = append(snapshot.Items, testsupport.MustRestoreItem(testsupport.ItemInput{
		SessionID: "ses_1", RunID: "run_1", ID: "item_tool", Kind: transcript.ToolCall,
		Status: transcript.ItemCompleted, OccurredAt: at, FinishedAt: at,
		Tool: &transcript.ToolInvocation{
			Name: "shell", Result: &result, Offload: &toolresult.Ref{ID: "BLOB234"},
		},
	}))
	snapshot.ToolResults = []toolresult.Blob{{
		ID: "BLOB234", SessionID: "ses_1", ItemID: "item_tool", ToolName: "shell",
		Preview: "bounded preview", Body: "full body", CreatedAt: at,
	}}
	snapshot.Plan = []plan.Step{{Description: "keep ownership", Status: plan.StatusPending}}

	portable, err := snapshot.PortableSnapshot()
	if err != nil {
		t.Fatalf("PortableSnapshot: %v", err)
	}
	snapshot.Items[0] = transcript.Item{}
	snapshot.ToolResults[0].Body = "source mutation"
	snapshot.Plan[0].Description = "source mutation"
	if portable.Items[0].ID() != "item_1" {
		t.Fatalf("portable Item changed with source slice: %+v", portable.Items[0])
	}
	if portable.ToolResults[0].Body != "full body" {
		t.Fatalf("portable tool result body = %q, want owned snapshot", portable.ToolResults[0].Body)
	}
	if portable.Plan[0].Description != "keep ownership" {
		t.Fatalf("portable Plan description = %q, want owned snapshot", portable.Plan[0].Description)
	}
}

func TestPortableSnapshotCanonicalSnapshotOwnsMessages(t *testing.T) {
	portable, err := portableSnapshotWithMessage().PortableSnapshot()
	if err != nil {
		t.Fatalf("PortableSnapshot: %v", err)
	}

	canonical, err := portable.CanonicalSnapshot()
	if err != nil {
		t.Fatalf("CanonicalSnapshot: %v", err)
	}
	portable.Messages[0].Parts[0].Text = "portable mutation"
	if text := canonical.Messages[0].Text(); text != "original" {
		t.Fatalf("canonical message after portable mutation = %q, want owned snapshot", text)
	}
	canonical.Messages[0].Parts[0].Text = "canonical mutation"
	if text := portable.Messages[0].Text(); text != "portable mutation" {
		t.Fatalf("portable message after canonical mutation = %q, want unchanged", text)
	}
}

func portableSnapshotWithMessage() Snapshot {
	snapshot := portableSnapshot()
	snapshot.Messages = []chat.Message{chat.NewUserMessage(chat.NewTextPart("original"))}
	root := snapshot.Runs[0].Snapshot()
	root.MessageMark = 1
	snapshot.Runs[0] = testsupport.MustRestoreRun(root)
	return snapshot
}

func TestSnapshotValidateToolResultsRejectsBrokenRelationships(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Snapshot)
		want   string
	}{
		{
			name: "missing blob",
			mutate: func(snapshot *Snapshot) {
				snapshot.ToolResults = nil
			},
			want: "references missing tool result",
		},
		{
			name: "detached blob",
			mutate: func(snapshot *Snapshot) {
				mutateSnapshotItem(snapshot, func(item *transcript.ItemSnapshot) { item.Tool.Offload = nil })
			},
			want: "references missing transcript item",
		},
		{
			name: "foreign session",
			mutate: func(snapshot *Snapshot) {
				snapshot.ToolResults[0].SessionID = "ses_other"
			},
			want: "belongs to session",
		},
		{
			name: "unrelated result",
			mutate: func(snapshot *Snapshot) {
				result := tool.StringResult("neither preview nor body")
				mutateSnapshotItem(snapshot, func(item *transcript.ItemSnapshot) { item.Tool.Result = &result })
			},
			want: "matches neither",
		},
		{
			name: "duplicate item binding",
			mutate: func(snapshot *Snapshot) {
				duplicate := snapshot.ToolResults[0]
				duplicate.ID = "OTHER234"
				snapshot.ToolResults = append(snapshot.ToolResults, duplicate)
			},
			want: "multiple tool results",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := offloadedSnapshot("full body")
			tt.mutate(&snapshot)
			if err := snapshot.ValidateToolResults(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateToolResults() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func mutateSnapshotItem(snapshot *Snapshot, mutate func(*transcript.ItemSnapshot)) {
	item := snapshot.Items[0].Snapshot()
	mutate(&item)
	restored, err := transcript.RestoreItem(item)
	if err != nil {
		panic(err)
	}
	snapshot.Items[0] = restored
}

func offloadedSnapshot(result string) Snapshot {
	ref := &toolresult.Ref{ID: "BLOB234"}
	value := tool.StringResult(result)
	return Snapshot{
		Session: testsupport.MustRestoreSession(session.Snapshot{ID: "ses_1"}),
		Items: []transcript.Item{testsupport.MustRestoreItem(testsupport.ItemInput{
			SessionID: "ses_1", ID: "item_1", Kind: transcript.ToolCall,
			Status: transcript.ItemCompleted,
			Tool:   &transcript.ToolInvocation{Name: "shell", Result: &value, Offload: ref},
		})},
		ToolResults: []toolresult.Blob{{
			ID: "BLOB234", SessionID: "ses_1", ItemID: "item_1", ToolName: "shell",
			Preview: "bounded preview", Body: "full body", CreatedAt: time.Unix(1, 0).UTC(),
		}},
	}
}

// The archive's run lineage carries rules a schema cannot state: JSON Schema
// cannot compare two fields, and "root" is the ABSENCE of the child edges, which
// no presence rule can condition on. So they are checked where the archive
// becomes a session — before anything is written.
func TestPortableSnapshotRefusesABrokenRunLineage(t *testing.T) {
	capabilities := run.Capabilities{}
	selection := mustTestSelection(t, "provider", "model")
	root := func() PortableRun {
		return PortableRun{
			SessionID: "ses_1", ID: "run_root", Outcome: run.OutcomeCompleted,
			Selection: selection, Capabilities: &capabilities,
		}
	}
	for name, runs := range map[string][]PortableRun{
		// A root with no capabilities is an archive that lost an admitted fact.
		// Defaulting it to empty would import a different Run.
		"root without capabilities": {
			{SessionID: "ses_1", ID: "run_root", Outcome: run.OutcomeCompleted},
		},
		// A child reads its root's contract; one of its own is a second statement of
		// something the archive already says once.
		"child with its own capabilities": {root(), {
			SessionID: "ses_1", ID: "run_child", Outcome: run.OutcomeCompleted,
			SpawnedByItemID: "item_1", ParentRunID: "run_root", RootRunID: "run_root",
			Capabilities: &capabilities,
		}},
		"child naming itself as its own root": {root(), {
			SessionID: "ses_1", ID: "run_child", Outcome: run.OutcomeCompleted,
			SpawnedByItemID: "item_1", ParentRunID: "run_root", RootRunID: "run_child",
		}},
		// A child whose root is not in the archive imports a tree that cannot be
		// walked — and a contract that cannot be read.
		"child whose root is absent": {{
			SessionID: "ses_1", ID: "run_child", Outcome: run.OutcomeCompleted,
			SpawnedByItemID: "item_1", ParentRunID: "run_gone", RootRunID: "run_gone",
		}},
	} {
		t.Run(name, func(t *testing.T) {
			portable := PortableSnapshot{
				Session: PortableSession{ID: "ses_1", Title: "t", CWD: "/w", Selection: selection},
				Runs:    runs,
			}
			if _, err := portable.CanonicalSnapshot(); !errors.Is(err, ErrInvalidPortableSnapshot) {
				t.Fatalf("CanonicalSnapshot err = %v, want ErrInvalidPortableSnapshot", err)
			}
		})
	}
}

func TestPortableSnapshotDelegatesModelIdentityToRun(t *testing.T) {
	selection := mustTestSelection(t, "provider", "model")
	capabilities := run.Capabilities{}
	at := time.Unix(1, 0).UTC()
	portable := PortableSnapshot{
		Session: PortableSession{
			ID: "ses_1", Title: "t", CWD: "/w", Selection: selection,
			CreatedAt: at, UpdatedAt: at,
		},
		Runs: []PortableRun{{
			SessionID: "ses_1", ID: "run_1", Outcome: run.OutcomeCompleted,
			Capabilities: &capabilities, CreatedAt: at, FinishedAt: at, UpdatedAt: at,
		}},
	}
	_, err := portable.CanonicalSnapshot()
	if !errors.Is(err, ErrInvalidPortableSnapshot) || !strings.Contains(err.Error(), "model selection is required") {
		t.Fatalf("CanonicalSnapshot without Run model identity error = %v", err)
	}
}

// A child inherits rather than restating its root's capabilities, so the
// restored Run must carry the root value rather than an empty set.
func TestPortableSnapshotChildInheritsRootCapabilities(t *testing.T) {
	capabilities := run.Capabilities{
		ChildRuns:      true,
		InterruptKinds: []interrupt.Kind{interrupt.Approval},
	}
	at := time.Unix(1, 0).UTC()
	selection := mustTestSelection(t, "provider", "model")
	portable := PortableSnapshot{
		Session: PortableSession{
			ID: "ses_1", Title: "t", CWD: "/w", Selection: selection,
			CreatedAt: at, UpdatedAt: at,
		},
		// The spawning item has to exist: a child run is spawned BY something, and an
		// archive naming an item it does not contain is a tree that cannot be walked.
		// The spawning item is a TOOL CALL: a child run is the execution of one.
		Items: []transcript.Item{testsupport.MustRestoreItem(testsupport.ItemInput{
			SessionID: "ses_1", RunID: "run_root", ID: "item_1", OccurredAt: at,
			FinishedAt: at,
			Status:     transcript.ItemCompleted, Kind: transcript.ToolCall,
			Tool: &transcript.ToolInvocation{Name: "delegate_task"},
		})},
		Runs: []PortableRun{
			{
				SessionID: "ses_1", ID: "run_root", Outcome: run.OutcomeCompleted,
				Selection: selection, Capabilities: &capabilities,
				CreatedAt: at, FinishedAt: at, UpdatedAt: at,
			},
			{
				SessionID: "ses_1", ID: "run_child", Outcome: run.OutcomeCompleted,
				SpawnedByItemID: "item_1", ParentRunID: "run_root", RootRunID: "run_root",
				Selection: selection, CreatedAt: at, FinishedAt: at, UpdatedAt: at,
			},
		},
	}
	snapshot, err := portable.CanonicalSnapshot()
	if err != nil {
		t.Fatalf("CanonicalSnapshot: %v", err)
	}
	for _, run := range snapshot.Runs {
		capabilities := run.Capabilities()
		if !capabilities.ChildRuns || len(capabilities.InterruptKinds) != 1 {
			t.Fatalf("run %q capabilities = %+v, want the root's", run.ID(), capabilities)
		}
	}
}
