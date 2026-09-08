package agent

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/exactint"
	"github.com/Tangerg/flame/runtime/protocol"
)

func testWorkspace(path string) protocol.WorkspaceInfo {
	return protocol.WorkspaceInfo{Ref: protocol.WorkspaceRef{Path: path}, ProjectRoot: path, Availability: protocol.WorkspaceAvailable}
}

const (
	testSessionProvider = "mock"
	testSessionModel    = "balanced"
)

func testPlan(t testing.TB, revision uint64, steps []protocol.PlanStep) *protocol.Plan {
	t.Helper()
	for index := range steps {
		if steps[index].ID == "" {
			steps[index].ID = fmt.Sprintf("step_%d", index+1)
		}
	}
	plan := &protocol.Plan{SessionID: "ses_1", State: &protocol.PlanState{
		Revision: revision, Steps: steps, UpdatedAt: time.Unix(1, 0).UTC(),
	}}
	if err := protocol.ValidateWireTree(*plan); err != nil {
		t.Fatalf("new test Plan: %v", err)
	}
	return plan
}

func testPlanChanged(t testing.TB, revision uint64, steps []protocol.PlanStep) PlanChanged {
	t.Helper()
	return PlanChanged{Plan: *testPlan(t, revision, steps)}
}

func TestSessionQueryNormalizesLocalFilterIdentity(t *testing.T) {
	t.Parallel()

	pageSize, err := NewPageSize(20)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := (SessionQuery{Search: "  release notes  ", Workspace: "  /repo/work  ", PageSize: pageSize}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	rows, rowsErr := normalized.PageSize.Rows()
	if normalized.Search != "release notes" || normalized.Workspace != "/repo/work" || rowsErr != nil || rows != 20 {
		t.Fatalf("normalized query = %+v", normalized)
	}
	for _, query := range []SessionQuery{
		{PageSize: PageSize{kind: explicitPageSize, rows: -1}},
		{PageSize: DefaultPageSize(), Workspace: "relative/workspace"},
		{PageSize: DefaultPageSize(), Workspace: "/repo/../repo"},
		{PageSize: DefaultPageSize(), Search: strings.Repeat("x", 1025)},
		{PageSize: DefaultPageSize(), Search: "bad\x00query"},
		{PageSize: DefaultPageSize(), Search: string([]byte{0xff})},
	} {
		if _, err := query.Normalize(); err == nil {
			t.Fatalf("Normalize accepted %+v", query)
		}
	}
}

func TestSessionSnapshotRestoresDurableProjection(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: protocol.Session{ID: "ses_1", Status: protocol.SessionStatusWaiting, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo"), Revision: 2},
		Transcript: []Block{
			{ID: "user_1", RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockUser, Text: "hello"},
			{ID: "tool_1", RunID: "run_1", Status: BlockStatusRunning, Kind: BlockTool, Tool: &ToolCall{Kind: ToolEdit, Name: "edit", Status: ToolRunning}},
		},
		Plan: testPlan(t, 3, []protocol.PlanStep{{Description: "inspect", Status: protocol.PlanStatusInProgress}}),
		Runs: []protocol.RunRef{protocol.RunRef{RunSummary: protocol.RunSummary{ID: "run_1", SessionID: "ses_1", Status: protocol.RunStatusWaiting}}},
		Interactions: []Interaction{Approval{
			RunID: "run_1", ItemID: "tool_1", Title: "edit", Rememberable: true,
			Tool: &ToolCall{Kind: ToolEdit, Name: "edit", Status: ToolRunning},
		}},
	}

	conversation := NewConversation()
	conversation.RestoreSnapshot(snapshot)
	if conversation.Phase() != ConversationWaiting || len(conversation.Blocks()) != 2 || len(conversation.Interactions()) != 1 {
		t.Fatalf("restored conversation = phase %v, blocks %d, interactions %d", conversation.Phase(), len(conversation.Blocks()), len(conversation.Interactions()))
	}
}

func TestSessionUpdateRequiresIdentityAndAtLeastOneValidField(t *testing.T) {
	title, workspace := "Title", "/workspace"
	for _, test := range []struct {
		name   string
		update UpdateSession
		valid  bool
	}{
		{name: "title", update: UpdateSession{SessionID: "ses_1", Title: &title, ExpectedRevision: 1}, valid: true},
		{name: "workspace", update: UpdateSession{SessionID: "ses_1", Workspace: &workspace, ExpectedRevision: 1}, valid: true},
		{name: "missing revision", update: UpdateSession{SessionID: "ses_1", Title: &title}},
		{name: "inexact revision", update: UpdateSession{SessionID: "ses_1", Title: &title, ExpectedRevision: exactint.Maximum + 1}},
		{name: "empty", update: UpdateSession{SessionID: "ses_1"}},
		{name: "missing identity", update: UpdateSession{Title: &title}},
		{name: "blank workspace", update: UpdateSession{SessionID: "ses_1", Workspace: new(string)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.update.Validate()
			if (err == nil) != test.valid {
				t.Fatalf("Validate = %v, valid %t", err, test.valid)
			}
		})
	}
}

func TestModelRefRoundTripKeepsProviderAndSlashBearingModel(t *testing.T) {
	want, err := NewModelRef("openrouter", "anthropic/claude-sonnet")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseModelRef(want.String())
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("ParseModelRef(%q) = %+v, want %+v", want.String(), got, want)
	}
	if _, err := NewModelRef("open/router", "model"); err == nil {
		t.Fatal("NewModelRef accepted a provider containing the identity separator")
	}
}

func TestSessionMutationsRejectInvalidInput(t *testing.T) {
	for _, invalid := range []interface{ Validate() error }{
		CreateSession{Workspace: "relative"},
		CreateSession{Title: "   "},
		ForkSession{},
		ForkSession{SessionID: "ses_source", FromRunID: "   "},
	} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("invalid session mutation %#v was accepted", invalid)
		}
	}
}

func TestSessionSnapshotRestoresAChildOwnedInterrupt(t *testing.T) {
	root := protocol.RunRef{RunSummary: protocol.RunSummary{ID: "run_root", SessionID: "ses_1", Status: protocol.RunStatusWaiting}}
	child := protocol.RunRef{RunSummary: protocol.RunSummary{ID: "run_child", SessionID: "ses_1", Status: protocol.RunStatusWaiting, SpawnedByItemID: "delegate", ParentRunID: root.ID, RootRunID: root.ID}}
	approval := Approval{
		RunID: child.ID, ItemID: "approval", Title: "Read generated output",
		Tool: &ToolCall{Kind: ToolRead, Name: "read", Status: ToolRunning},
	}
	snapshot := SessionSnapshot{
		Session: protocol.Session{ID: "ses_1", Status: protocol.SessionStatusWaiting, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo"), Revision: 1},
		Runs:    []protocol.RunRef{root, child},
		Transcript: []Block{
			{ID: "delegate", RunID: root.ID, Status: BlockStatusRunning, Kind: BlockTool, Tool: &ToolCall{Kind: ToolTask, Name: "delegate_task", Status: ToolRunning}},
			{ID: approval.ItemID, RunID: child.ID, Status: BlockStatusRunning, Kind: BlockTool, Tool: approval.Tool},
		},
		Interactions: []Interaction{approval},
	}

	active, ok := snapshot.ActiveRun()
	if !ok || active.ID != root.ID {
		t.Fatalf("ActiveRun = %+v, %v", active, ok)
	}
	conversation := NewConversation()
	conversation.RestoreSnapshot(snapshot)
	if conversation.RunID() != root.ID || conversation.Interactions()[0].(Approval).RunID != child.ID {
		t.Fatalf("restored tree = root %s interactions %+v", conversation.RunID(), conversation.Interactions())
	}
}

func TestSessionSnapshotRestoresLatestFinishedRun(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: protocol.Session{ID: "ses_1", Status: protocol.SessionStatusIdle, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo"), Revision: 1},
		Runs:    []protocol.RunRef{protocol.RunRef{RunSummary: protocol.RunSummary{ID: "run_1", SessionID: "ses_1", Status: protocol.RunStatusFinished, Outcome: (Outcome{Status: protocol.OutcomeCompleted}).RunOutcome()}, Metrics: protocol.RunMetrics{Usage: &protocol.Usage{ModelUsage: protocol.ModelUsage{InputTokens: 12, OutputTokens: 3}}}}},
	}
	conversation := NewConversation()
	conversation.RestoreSnapshot(snapshot)
	if conversation.Phase() != ConversationIdle || conversation.RunID() != "run_1" || conversation.Outcome().Status != protocol.OutcomeCompleted || conversation.Usage().InputTokens != 12 {
		t.Fatalf("restored finished conversation = phase %v, run %q, outcome %+v, usage %+v", conversation.Phase(), conversation.RunID(), conversation.Outcome(), conversation.Usage())
	}
}

func TestConversationRestoresCursorlessAttachmentHead(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: protocol.Session{ID: "ses_1", Status: protocol.SessionStatusRunning, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo"), Revision: 1},
		Runs:    []protocol.RunRef{protocol.RunRef{RunSummary: protocol.RunSummary{ID: "run_1", SessionID: "ses_1", Status: protocol.RunStatusRunning}, ActiveSegmentID: "seg_1"}},
	}
	stream := SegmentStream{
		RunID: "run_1", SegmentID: "seg_1", HeadEventID: "opaque-head",
		Events: func(func(RunEvent, error) bool) {},
	}
	conversation := NewConversation()
	if err := conversation.RestoreAttachedSnapshot(snapshot, stream); err != nil {
		t.Fatal(err)
	}
	if conversation.Checkpoint() != "opaque-head" || conversation.Phase() != ConversationRunning {
		t.Fatalf("restored checkpoint %q, phase %v", conversation.Checkpoint(), conversation.Phase())
	}

	stream.SegmentID = "seg_other"
	if err := conversation.RestoreAttachedSnapshot(snapshot, stream); err == nil {
		t.Fatal("mismatched attached stream was accepted")
	}
}

func TestSessionSnapshotFindsTheLastDurableAssistantText(t *testing.T) {
	snapshot := SessionSnapshot{Transcript: []Block{
		{Kind: BlockAssistant, Text: "first"},
		{Kind: BlockReasoning, Text: "internal"},
		{Kind: BlockAssistant, Text: "  final answer  \n"},
	}}
	text, err := snapshot.LastAssistantText()
	if err != nil || text != "  final answer  \n" {
		t.Fatalf("LastAssistantText = (%q, %v)", text, err)
	}
}

func TestConversationMatchesColdSnapshotSemantics(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: protocol.Session{ID: "ses_1", Title: "Original", Status: protocol.SessionStatusIdle, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo"), Revision: 1},
		Transcript: []Block{{
			ID: "answer_1", RunID: "run_1", Status: BlockStatusCompleted,
			Kind: BlockAssistant, Text: "done",
		}},
		Runs: []protocol.RunRef{protocol.RunRef{RunSummary: protocol.RunSummary{ID: "run_1", SessionID: "ses_1", Status: protocol.RunStatusFinished, Outcome: (Outcome{Status: protocol.OutcomeCompleted}).RunOutcome()}, Metrics: protocol.RunMetrics{Usage: &protocol.Usage{ModelUsage: protocol.ModelUsage{InputTokens: 5}}}}},
		Plan: testPlan(t, 2, []protocol.PlanStep{{Description: "inspect", Status: protocol.PlanStatusCompleted}}),
	}
	conversation := NewConversation()
	conversation.RestoreSnapshot(snapshot)

	snapshot.Session.Title = "Renamed elsewhere"
	if !conversation.MatchesSnapshot(snapshot) {
		t.Fatal("session metadata changed the conversation identity")
	}

	tests := []struct {
		name   string
		mutate func(*SessionSnapshot)
	}{
		{name: "transcript", mutate: func(value *SessionSnapshot) { value.Transcript[0].Text = "changed" }},
		{name: "plan", mutate: func(value *SessionSnapshot) {
			steps := slices.Clone(value.Plan.State.Steps)
			steps[0].Status = protocol.PlanStatusInProgress
			value.Plan = testPlan(t, value.Plan.State.Revision, steps)
		}},
		{name: "usage", mutate: func(value *SessionSnapshot) { value.Runs[0].Metrics.Usage.InputTokens++ }},
		{name: "outcome", mutate: func(value *SessionSnapshot) { value.Runs[0].Outcome.Type = protocol.OutcomeCanceled }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := snapshot
			changed.Transcript = cloneBlocks(snapshot.Transcript)
			changed.Plan = clonePlan(snapshot.Plan)
			changed.Runs = []protocol.RunRef{CloneRun(snapshot.Runs[0])}
			test.mutate(&changed)
			if conversation.MatchesSnapshot(changed) {
				t.Fatal("semantic change matched the live conversation")
			}
		})
	}

}
