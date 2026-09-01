package agent

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/cli/internal/exactint"
	"github.com/Tangerg/flame/runtime/protocol"
)

func testWorkspace(path string) workspace.Workspace {
	return workspace.Workspace{Path: path, ProjectRoot: path, Availability: workspace.Available}
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
		{PageSize: DefaultPageSize(), Search: strings.Repeat("x", MaximumSessionSearchCharacters+1)},
		{PageSize: DefaultPageSize(), Search: "bad\x00query"},
		{PageSize: DefaultPageSize(), Search: string([]byte{0xff})},
	} {
		if _, err := query.Normalize(); err == nil {
			t.Fatalf("Normalize accepted %+v", query)
		}
	}
}

func TestSessionEqualityUsesDurableTimeSemantics(t *testing.T) {
	created := time.Date(2026, time.August, 13, 8, 30, 0, 0, time.FixedZone("source", 8*60*60))
	updated := created.Add(time.Minute)
	session := Session{
		ID: "ses_1", Title: "Review", Status: SessionIdle,
		Provider: "deepseek", Model: "deepseek-v4-flash",
		Workspace: testWorkspace("/tmp/demo"), CreatedAt: created, UpdatedAt: updated,
		Favorite: true, Revision: 3,
	}
	equivalent := session
	equivalent.CreatedAt = created.UTC()
	equivalent.UpdatedAt = updated.UTC()
	if !session.Equal(equivalent) {
		t.Fatal("equal instants with different locations changed session identity")
	}
	equivalent.Revision++
	if session.Equal(equivalent) {
		t.Fatal("a durable session revision change compared equal")
	}
	equivalent = session
	equivalent.ReasoningEffort = "high"
	if session.Equal(equivalent) {
		t.Fatal("a reasoning-effort change compared equal")
	}
}

func TestSessionRevisionStaysInsideTheExactJSONEnvelope(t *testing.T) {
	session := Session{
		ID: "ses_1", Status: SessionIdle, Provider: testSessionProvider, Model: testSessionModel,
		Workspace: testWorkspace("/tmp/demo"), Revision: exactint.Maximum,
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("maximum exact revision: %v", err)
	}
	session.Revision++
	if err := session.Validate(); err == nil {
		t.Fatal("Session accepted an inexact JSON revision")
	}
}

func TestSessionRejectsNonExactIdentity(t *testing.T) {
	session := Session{
		ID: " ses_1", Status: SessionIdle, Provider: testSessionProvider, Model: testSessionModel,
		Workspace: testWorkspace("/tmp/demo"), Revision: 1,
	}
	if err := session.Validate(); err == nil {
		t.Fatal("Session accepted an identity that requires trimming")
	}
}

func TestSessionRejectsReasoningEffortWithoutAModel(t *testing.T) {
	session := Session{
		ID: "ses_1", Status: SessionIdle, ReasoningEffort: "high",
		Workspace: testWorkspace("/tmp/demo"), Revision: 1,
	}
	if err := session.Validate(); err == nil {
		t.Fatal("Session accepted reasoning effort without a provider and model")
	}
}

func TestSessionSnapshotRestoresDurableProjection(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: Session{ID: "ses_1", Status: SessionWaiting, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo"), Revision: 2},
		Transcript: []Block{
			{ID: "user_1", RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockUser, Text: "hello"},
			{ID: "tool_1", RunID: "run_1", Status: BlockStatusRunning, Kind: BlockTool, Tool: &ToolCall{Kind: ToolEdit, Name: "edit", Status: ToolRunning}},
		},
		Plan: testPlan(t, 3, []protocol.PlanStep{{Description: "inspect", Status: protocol.PlanStatusInProgress}}),
		Runs: []Run{testRootRun(Run{ID: "run_1", SessionID: "ses_1", Status: RunStatusWaiting})},
		Interactions: []Interaction{Approval{
			RunID: "run_1", ItemID: "tool_1", Title: "edit", Rememberable: true,
			Tool: &ToolCall{Kind: ToolEdit, Name: "edit", Status: ToolRunning},
		}},
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation()
	if err := conversation.RestoreSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	if conversation.Phase() != ConversationWaiting || len(conversation.Blocks()) != 2 || len(conversation.Interactions()) != 1 {
		t.Fatalf("restored conversation = phase %v, blocks %d, interactions %d", conversation.Phase(), len(conversation.Blocks()), len(conversation.Interactions()))
	}
}

func TestSessionSnapshotRejectsWaitingWithoutInteractions(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: Session{ID: "ses_1", Status: SessionWaiting, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo"), Revision: 1},
		Runs:    []Run{testRootRun(Run{ID: "run_1", SessionID: "ses_1", Status: RunStatusWaiting})},
	}
	if err := snapshot.Validate(); err == nil {
		t.Fatal("waiting snapshot without interactions was accepted")
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

func TestSessionUpdateResultMustFulfillTheCommand(t *testing.T) {
	title, path, favorite := "Renamed", "/workspace/new", true
	model := ModelRef{Provider: "provider-new", Model: "model-new"}
	update := UpdateSession{
		SessionID: "ses_1", Title: &title, Workspace: &path, Model: &model,
		Favorite: &favorite, ExpectedRevision: 4,
	}
	valid := Session{
		ID: "ses_1", Title: title, Status: SessionIdle, Provider: model.Provider, Model: model.Model,
		Workspace: testWorkspace(path), Favorite: favorite, Revision: 5,
	}
	if err := update.ValidateResult(valid); err != nil {
		t.Fatalf("valid result: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Session)
		want   string
	}{
		{name: "identity", mutate: func(result *Session) { result.ID = "ses_2" }, want: "runtime returned session"},
		{name: "revision", mutate: func(result *Session) { result.Revision = 4 }, want: "runtime returned revision"},
		{name: "revision jump", mutate: func(result *Session) { result.Revision = 6 }, want: "runtime returned revision"},
		{name: "title", mutate: func(result *Session) { result.Title = "Old" }, want: "runtime returned title"},
		{name: "workspace", mutate: func(result *Session) { result.Workspace = testWorkspace("/workspace/old") }, want: "runtime returned workspace"},
		{name: "model", mutate: func(result *Session) { result.Model = "model-old" }, want: "runtime returned model"},
		{name: "favorite", mutate: func(result *Session) { result.Favorite = false }, want: "runtime returned favorite"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := valid
			test.mutate(&result)
			err := update.ValidateResult(result)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateResult error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSessionCreationAndForkResultsMustFulfillTheCommand(t *testing.T) {
	created := Session{
		ID: "ses_new", Title: "Requested", Status: SessionIdle,
		Provider: testSessionProvider, Model: testSessionModel,
		Workspace: testWorkspace("/workspace"), Revision: 1,
	}
	create := CreateSession{Title: created.Title, Workspace: created.Workspace.Path}
	if err := create.ValidateResult(created); err != nil {
		t.Fatalf("valid create result: %v", err)
	}
	wrongCreate := created
	wrongCreate.Workspace = testWorkspace("/other")
	if err := create.ValidateResult(wrongCreate); err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("create result error = %v", err)
	}
	nonInitialCreate := created
	nonInitialCreate.Revision = 2
	if err := create.ValidateResult(nonInitialCreate); err == nil || !strings.Contains(err.Error(), "initial revision") {
		t.Fatalf("create initial revision error = %v", err)
	}

	fork := ForkSession{SessionID: "ses_source", Title: created.Title}
	if err := fork.ValidateResult(created); err != nil {
		t.Fatalf("valid fork result: %v", err)
	}
	if err := fork.ValidateResult(nonInitialCreate); err == nil || !strings.Contains(err.Error(), "initial revision") {
		t.Fatalf("fork initial revision error = %v", err)
	}
	wrongFork := created
	wrongFork.ID = fork.SessionID
	if err := fork.ValidateResult(wrongFork); err == nil || !strings.Contains(err.Error(), "source session") {
		t.Fatalf("fork result error = %v", err)
	}

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
	root := testRootRun(Run{ID: "run_root", SessionID: "ses_1", Status: RunStatusWaiting})
	child := testChildRun(Run{
		ID: "run_child", SessionID: "ses_1", Status: RunStatusWaiting,
		Lineage: testChildRunLineage(t, "run_child", "delegate", root.ID, root.ID),
	})
	approval := Approval{
		RunID: child.ID, ItemID: "approval", Title: "Read generated output",
		Tool: &ToolCall{Kind: ToolRead, Name: "read", Status: ToolRunning},
	}
	snapshot := SessionSnapshot{
		Session: Session{ID: "ses_1", Status: SessionWaiting, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo"), Revision: 1},
		Runs:    []Run{root, child},
		Transcript: []Block{
			{ID: "delegate", RunID: root.ID, Status: BlockStatusRunning, Kind: BlockTool, Tool: &ToolCall{Kind: ToolTask, Name: "delegate_task", Status: ToolRunning}},
			{ID: approval.ItemID, RunID: child.ID, Status: BlockStatusRunning, Kind: BlockTool, Tool: approval.Tool},
		},
		Interactions: []Interaction{approval},
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	active, ok := snapshot.ActiveRun()
	if !ok || active.ID != root.ID {
		t.Fatalf("ActiveRun = %+v, %v", active, ok)
	}
	conversation := NewConversation()
	if err := conversation.RestoreSnapshot(snapshot); err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}
	if conversation.RunID() != root.ID || conversation.Interactions()[0].(Approval).RunID != child.ID {
		t.Fatalf("restored tree = root %s interactions %+v", conversation.RunID(), conversation.Interactions())
	}
}

func TestSessionSnapshotRestoresLatestFinishedRun(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: Session{ID: "ses_1", Status: SessionIdle, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo"), Revision: 1},
		Runs: []Run{testRootRun(Run{
			ID: "run_1", SessionID: "ses_1", Status: RunStatusFinished,
			Outcome: Outcome{Status: OutcomeCompleted}, Usage: Usage{InputTokens: 12, OutputTokens: 3},
		})},
	}
	conversation := NewConversation()
	if err := conversation.RestoreSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	if conversation.Phase() != ConversationIdle || conversation.RunID() != "run_1" || conversation.Outcome().Status != OutcomeCompleted || conversation.Usage().InputTokens != 12 {
		t.Fatalf("restored finished conversation = phase %v, run %q, outcome %+v, usage %+v", conversation.Phase(), conversation.RunID(), conversation.Outcome(), conversation.Usage())
	}
}

func TestSessionSnapshotRejectsLifecycleDrift(t *testing.T) {
	lineage := testChildRunLineage(t, "run_child", "delegate", "run_root", "run_root")
	for _, test := range []struct {
		name     string
		snapshot SessionSnapshot
		want     string
	}{
		{
			name: "running run with idle session",
			snapshot: SessionSnapshot{
				Session: Session{ID: "ses_1", Status: SessionIdle, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo"), Revision: 1},
				Runs:    []Run{testRootRun(Run{ID: "run_1", SessionID: "ses_1", Status: RunStatusRunning, ActiveSegmentID: "seg_1"})},
			},
		},
		{
			name: "active run before latest run",
			snapshot: SessionSnapshot{
				Session: Session{ID: "ses_1", Status: SessionRunning, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo"), Revision: 1},
				Runs: []Run{
					testRootRun(Run{ID: "run_1", SessionID: "ses_1", Status: RunStatusRunning, ActiveSegmentID: "seg_1"}),
					testRootRun(Run{ID: "run_2", SessionID: "ses_1", Status: RunStatusFinished, Outcome: Outcome{Status: OutcomeCompleted}}),
				},
			},
		},
		{
			name: "waiting child beneath running root",
			want: "waiting beneath running root",
			snapshot: SessionSnapshot{
				Session: Session{ID: "ses_1", Status: SessionRunning, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo"), Revision: 1},
				Runs: []Run{
					testRootRun(Run{ID: "run_root", SessionID: "ses_1", Status: RunStatusRunning, ActiveSegmentID: "seg_root"}),
					testChildRun(Run{ID: "run_child", SessionID: "ses_1", Lineage: lineage, Status: RunStatusWaiting}),
				},
			},
		},
		{
			name: "running child beneath waiting root",
			want: "running beneath waiting root",
			snapshot: SessionSnapshot{
				Session: Session{ID: "ses_1", Status: SessionWaiting, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo"), Revision: 1},
				Runs: []Run{
					testRootRun(Run{ID: "run_root", SessionID: "ses_1", Status: RunStatusWaiting}),
					testChildRun(Run{ID: "run_child", SessionID: "ses_1", Lineage: lineage, Status: RunStatusRunning, ActiveSegmentID: "seg_child"}),
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.snapshot.Validate()
			if err == nil {
				t.Fatal("inconsistent snapshot was accepted")
			}
			if test.want != "" && !strings.Contains(err.Error(), test.want) {
				t.Fatalf("snapshot error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSessionSnapshotRejectsRunningItemsWithoutAnActiveRun(t *testing.T) {
	snapshot := SessionSnapshot{
		Session:    Session{ID: "ses_1", Status: SessionIdle, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo")},
		Transcript: []Block{{ID: "tool_1", RunID: "run_1", Status: BlockStatusRunning, Kind: BlockTool, Tool: &ToolCall{Kind: ToolShell, Name: "shell", Status: ToolRunning}}},
	}
	if err := snapshot.Validate(); err == nil {
		t.Fatal("idle snapshot with a running item was accepted")
	}
}

func TestSessionSnapshotRejectsTransientRunningItems(t *testing.T) {
	for _, kind := range []BlockKind{BlockAssistant, BlockReasoning} {
		t.Run(string(kind), func(t *testing.T) {
			snapshot := SessionSnapshot{
				Session:    Session{ID: "ses_1", Status: SessionRunning, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo")},
				Transcript: []Block{{ID: "preview_1", RunID: "run_1", Status: BlockStatusRunning, Kind: kind}},
				Runs:       []Run{testRootRun(Run{ID: "run_1", SessionID: "ses_1", Status: RunStatusRunning, ActiveSegmentID: "seg_1"})},
			}
			if err := snapshot.Validate(); err == nil {
				t.Fatalf("snapshot accepted a durable running %s preview", kind)
			}
		})
	}
}

func TestSessionSnapshotRejectsItemWithoutItsRun(t *testing.T) {
	snapshot := SessionSnapshot{
		Session:    Session{ID: "ses_1", Status: SessionIdle, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo")},
		Transcript: []Block{{ID: "message_1", RunID: "run_missing", Status: BlockStatusCompleted, Kind: BlockAssistant, Text: "orphaned"}},
	}
	if err := snapshot.Validate(); err == nil {
		t.Fatal("snapshot with an orphaned item was accepted")
	}
}

func TestConversationRestoresCursorlessAttachmentHead(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: Session{ID: "ses_1", Status: SessionRunning, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo"), Revision: 1},
		Runs:    []Run{testRootRun(Run{ID: "run_1", SessionID: "ses_1", Status: RunStatusRunning, ActiveSegmentID: "seg_1"})},
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
		{Kind: BlockAssistant, Text: "  final answer  "},
	}}
	text, err := snapshot.LastAssistantText()
	if err != nil || text != "final answer" {
		t.Fatalf("LastAssistantText = (%q, %v)", text, err)
	}
}

func TestConversationMatchesColdSnapshotSemantics(t *testing.T) {
	snapshot := SessionSnapshot{
		Session: Session{ID: "ses_1", Title: "Original", Status: SessionIdle, Provider: testSessionProvider, Model: testSessionModel, Workspace: testWorkspace("/tmp/demo"), Revision: 1},
		Transcript: []Block{{
			ID: "answer_1", RunID: "run_1", Status: BlockStatusCompleted,
			Kind: BlockAssistant, Text: "done",
		}},
		Runs: []Run{testRootRun(Run{
			ID: "run_1", SessionID: "ses_1", Status: RunStatusFinished,
			Outcome: Outcome{Status: OutcomeCompleted}, Usage: Usage{InputTokens: 5},
		})},
		Plan: testPlan(t, 2, []protocol.PlanStep{{Description: "inspect", Status: protocol.PlanStatusCompleted}}),
	}
	conversation := NewConversation()
	if err := conversation.RestoreSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}

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
		{name: "usage", mutate: func(value *SessionSnapshot) { value.Runs[0].Usage.InputTokens++ }},
		{name: "outcome", mutate: func(value *SessionSnapshot) { value.Runs[0].Outcome.Status = OutcomeCanceled }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := snapshot
			changed.Transcript = cloneBlocks(snapshot.Transcript)
			changed.Plan = clonePlan(snapshot.Plan)
			changed.Runs = []Run{snapshot.Runs[0].Clone()}
			test.mutate(&changed)
			if conversation.MatchesSnapshot(changed) {
				t.Fatal("semantic change matched the live conversation")
			}
		})
	}

	invalid := snapshot
	invalid.Session.Status = "broken"
	if conversation.MatchesSnapshot(invalid) {
		t.Fatal("invalid snapshot matched the live conversation")
	}
}
