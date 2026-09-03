package protocol

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func agentMemoryWireItemID(digit byte) string {
	return agentmemory.ItemIDPrefix + strings.Repeat(
		string(digit),
		agentmemory.MaximumItemIDCharacters-len(agentmemory.ItemIDPrefix),
	)
}

func TestRuntimeEventWireConstraints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event RuntimeEvent
		field string
	}{{
		name: "valid files change",
		event: RuntimeEvent{
			Type: RuntimeFilesChanged, Sequence: 1, Paths: []string{"README.md"},
		},
	}, {
		name: "sequence starts at one",
		event: RuntimeEvent{
			Type: RuntimeSkillsChanged, Sequence: 0,
		},
		field: "sequence",
	}, {
		name: "files change needs a concrete path",
		event: RuntimeEvent{
			Type: RuntimeFilesChanged, Sequence: 1, Paths: []string{},
		},
		field: "paths",
	}, {
		name: "resync names its recovery scope",
		event: RuntimeEvent{
			Type: RuntimeResync, Sequence: 1,
		},
		field: "topics",
	}, {
		name: "resync scope is not empty",
		event: RuntimeEvent{
			Type: RuntimeResync, Sequence: 1, Topics: []RuntimeTopic{},
		},
		field: "topics",
	}, {
		name: "optional scope is nonempty when present",
		event: RuntimeEvent{
			Type: RuntimeSessionsChanged, Sequence: 1, SessionIDs: []string{},
		},
		field: "sessionIds",
	}, {
		name: "variant fields stay closed",
		event: RuntimeEvent{
			Type: RuntimeSkillsChanged, Sequence: 1, RunIDs: []string{"run_1"},
		},
		field: "runIds",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.event.ValidateWire()
			if test.field == "" {
				if err != nil {
					t.Fatalf("ValidateWire rejected a valid event: %v", err)
				}
				return
			}
			assertConstraintField(t, err, "RuntimeEvent", test.field)
		})
	}
}

func TestRunProgressCarriesAtLeastOneValidFact(t *testing.T) {
	t.Parallel()

	assertConstraintField(
		t,
		(RunProgress{}).ValidateWire(),
		"RunProgress",
		"step|usage|contextTokens|activity",
	)
	negativeStep, negativeContext := -1, int64(-1)
	assertConstraintField(t, (RunProgress{Step: &negativeStep}).ValidateWire(), "RunProgress", "step")
	assertConstraintField(t, (RunProgress{ContextTokens: &negativeContext}).ValidateWire(), "RunProgress", "contextTokens")

	zeroStep, zeroContext := 0, int64(0)
	for _, progress := range []RunProgress{
		{Step: &zeroStep},
		{Usage: &Usage{}},
		{ContextTokens: &zeroContext},
		{Activity: "Calling model"},
	} {
		if err := progress.ValidateWire(); err != nil {
			t.Errorf("ValidateWire rejected valid progress %+v: %v", progress, err)
		}
	}
}

func TestSegmentFinishedCarriesFinalContextFootprint(t *testing.T) {
	t.Parallel()

	completed := SegmentOutcome{Type: SegmentOutcomeType(OutcomeCompleted)}
	metrics := RunMetrics{}
	missing := StreamEvent{
		Type: StreamSegmentFinished, Outcome: &completed, Metrics: &metrics,
	}
	assertConstraintField(t, missing.ValidateWire(), "StreamEvent", "contextTokens")

	negative := int64(-1)
	invalid := missing
	invalid.ContextTokens = &negative
	assertConstraintField(t, invalid.ValidateWire(), "StreamEvent", "contextTokens")

	zero := int64(0)
	valid := missing
	valid.ContextTokens = &zero
	if err := valid.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected a complete segment boundary: %v", err)
	}
}

func TestPlanUsesAnOptionalCommittedStateInsteadOfRevisionZero(t *testing.T) {
	t.Parallel()

	unwritten := Plan{SessionID: "ses_1"}
	if err := unwritten.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected unwritten Plan: %v", err)
	}
	assertConstraintField(t, (PlanState{}).ValidateWire(), "PlanState", "revision")

	committed := Plan{
		SessionID: "ses_1",
		State: &PlanState{
			Revision: 1, Steps: []PlanStep{}, UpdatedAt: time.Unix(1, 0).UTC(),
		},
	}
	if err := ValidateWireTree(committed); err != nil {
		t.Fatalf("ValidateWireTree rejected committed empty Plan: %v", err)
	}
	committed.State.Steps = []PlanStep{{ID: "step-1", Description: " \t", Status: PlanStatusPending}}
	assertConstraintField(t, ValidateWireTree(committed), "Plan", "state.steps[0].description")

	updatedWithoutState := StreamEvent{Type: StreamPlanUpdated, Plan: &unwritten}
	assertConstraintField(t, updatedWithoutState.ValidateWire(), "StreamEvent", "plan.state")
}

func TestAgentMemoryTargetIsUnambiguous(t *testing.T) {
	t.Parallel()

	workspace := &WorkspaceRef{Path: "/repo"}
	for _, request := range []AgentMemoryListRequest{
		{Scope: AgentMemoryScopeProject, Workspace: workspace},
		{Scope: AgentMemoryScopeUser},
	} {
		if err := request.ValidateWire(); err != nil {
			t.Errorf("ValidateWire rejected valid target %+v: %v", request, err)
		}
	}

	assertConstraintField(
		t,
		(AgentMemoryListRequest{Scope: AgentMemoryScopeProject}).ValidateWire(),
		"AgentMemoryListRequest",
		"workspace",
	)
	assertConstraintField(
		t,
		(AgentMemoryListRequest{Scope: AgentMemoryScopeUser, Workspace: workspace}).ValidateWire(),
		"AgentMemoryListRequest",
		"workspace",
	)
	assertConstraintField(
		t,
		(AgentMemoryAddRequest{Scope: AgentMemoryScopeUser, Workspace: workspace, Content: "fact"}).ValidateWire(),
		"AgentMemoryAddRequest",
		"workspace",
	)
}

func TestKnowledgeTargetIsUnambiguous(t *testing.T) {
	t.Parallel()

	workspace := &WorkspaceRef{Path: "/repo"}
	for _, request := range []WireValidator{
		GetKnowledgeRequest{Scope: KnowledgeScopeCWD, Workspace: workspace},
		GetKnowledgeRequest{Scope: KnowledgeScopeProjectRoot, Workspace: workspace},
		GetKnowledgeRequest{Scope: KnowledgeScopeHome},
		UpdateKnowledgeRequest{Scope: KnowledgeScopeCWD, Workspace: workspace, ExpectedRevision: "rev-1"},
		UpdateKnowledgeRequest{Scope: KnowledgeScopeHome, ExpectedRevision: "rev-1"},
	} {
		if err := request.ValidateWire(); err != nil {
			t.Errorf("ValidateWire rejected valid target %T: %v", request, err)
		}
	}

	for _, test := range []struct {
		shape   string
		request WireValidator
	}{
		{shape: "GetKnowledgeRequest", request: GetKnowledgeRequest{Scope: KnowledgeScopeCWD}},
		{shape: "GetKnowledgeRequest", request: GetKnowledgeRequest{Scope: KnowledgeScopeProjectRoot}},
		{shape: "GetKnowledgeRequest", request: GetKnowledgeRequest{Scope: KnowledgeScopeHome, Workspace: workspace}},
		{shape: "UpdateKnowledgeRequest", request: UpdateKnowledgeRequest{Scope: KnowledgeScopeHome, Workspace: workspace, ExpectedRevision: "rev-1"}},
	} {
		assertConstraintField(t, test.request.ValidateWire(), test.shape, "workspace")
	}
}

func TestRollbackFileRestorationRequiresRunBoundary(t *testing.T) {
	t.Parallel()

	for _, request := range []RollbackSessionRequest{
		{SessionID: "ses_1"},
		{SessionID: "ses_1", RestoreType: RestoreHistory},
		{SessionID: "ses_1", ToRunID: "run_1", RestoreType: RestoreFiles},
		{SessionID: "ses_1", ToRunID: "run_1", RestoreType: RestoreBoth},
	} {
		if err := request.ValidateWire(); err != nil {
			t.Errorf("ValidateWire rejected rollback target %+v: %v", request, err)
		}
	}
	for _, restore := range []RestoreType{RestoreFiles, RestoreBoth} {
		assertConstraintField(t, (RollbackSessionRequest{
			SessionID: "ses_1", RestoreType: restore,
		}).ValidateWire(), "RollbackSessionRequest", "toRunId")
	}
}

func TestAuthoringContextOutputsAreComplete(t *testing.T) {
	t.Parallel()

	for _, value := range []WireValidator{
		AgentDoc{Path: "/repo/AGENTS.md", Scope: AgentDocScopeProjectRoot},
		Recipe{Name: "review", Body: "Review $ARGUMENTS", Scope: RecipeScopeProject, Source: "/repo/review.md"},
	} {
		if err := value.ValidateWire(); err != nil {
			t.Errorf("ValidateWire rejected complete %T: %v", value, err)
		}
	}

	for _, test := range []struct {
		shape string
		field string
		value WireValidator
	}{
		{shape: "AgentDoc", field: "path", value: AgentDoc{Path: " \t", Scope: AgentDocScopeHome}},
		{shape: "Recipe", field: "name", value: Recipe{Body: "body", Scope: RecipeScopeGlobal, Source: "/recipe.md"}},
		{shape: "Recipe", field: "body", value: Recipe{Name: "review", Body: " \n\t", Scope: RecipeScopeGlobal, Source: "/recipe.md"}},
		{shape: "Recipe", field: "source", value: Recipe{Name: "review", Body: "body", Scope: RecipeScopeGlobal}},
	} {
		assertConstraintField(t, test.value.ValidateWire(), test.shape, test.field)
	}
}

func TestSkillWireConstraintsPublishCompleteProjectionIdentity(t *testing.T) {
	t.Parallel()

	const revision = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, value := range []WireValidator{
		Skill{Name: "review", Scope: SkillScopeProject},
		ManagedSkill{Name: "review", Lifecycle: SkillLifecycleActive},
		SkillProposal{
			Name: "review", Revision: revision, Scope: SkillScopeUser,
			Description: "Review code", Instructions: "Inspect the requested code.",
			Origin: SkillProposalOriginRequested,
		},
		SkillProposalRef{Name: "review", Revision: revision, Scope: SkillScopeUser},
	} {
		if err := value.ValidateWire(); err != nil {
			t.Errorf("ValidateWire rejected complete %T: %v", value, err)
		}
	}

	for _, test := range []struct {
		shape string
		field string
		value WireValidator
	}{
		{shape: "Skill", field: "name", value: Skill{Scope: SkillScopeProject}},
		{shape: "ManagedSkill", field: "name", value: ManagedSkill{Name: " \t", Lifecycle: SkillLifecycleActive}},
		{shape: "SkillProposal", field: "revision", value: SkillProposal{
			Name: "review", Revision: strings.ToUpper(revision), Scope: SkillScopeUser,
			Description: "Review code", Instructions: "Inspect code.",
		}},
		{shape: "SkillProposal", field: "description", value: SkillProposal{
			Name: "review", Revision: revision, Scope: SkillScopeUser, Instructions: "Inspect code.",
		}},
		{shape: "SkillProposal", field: "instructions", value: SkillProposal{
			Name: "review", Revision: revision, Scope: SkillScopeUser, Description: "Review code",
		}},
		{shape: "SkillProposalRef", field: "revision", value: SkillProposalRef{
			Name: "review", Revision: "short", Scope: SkillScopeUser,
		}},
	} {
		assertConstraintField(t, test.value.ValidateWire(), test.shape, test.field)
	}
}

func TestAgentMemoryContentWireConstraintUsesUnicodeCharacters(t *testing.T) {
	t.Parallel()

	content := strings.Repeat("界", agentmemory.MaxContentCharacters)
	if err := (AgentMemoryAddRequest{
		Scope: AgentMemoryScopeUser, Content: content,
	}).ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected the memory content boundary: %v", err)
	}

	content += "界"
	assertConstraintField(
		t,
		(AgentMemoryAddRequest{Scope: AgentMemoryScopeUser, Content: content}).ValidateWire(),
		"AgentMemoryAddRequest",
		"content",
	)
	assertConstraintField(
		t,
		(AgentMemoryUpdateRequest{ID: agentMemoryWireItemID('1'), Content: &content}).ValidateWire(),
		"AgentMemoryUpdateRequest",
		"content",
	)
	for _, blank := range []string{"", " \n\t"} {
		assertConstraintField(
			t,
			(AgentMemoryUpdateRequest{ID: agentMemoryWireItemID('1'), Content: &blank}).ValidateWire(),
			"AgentMemoryUpdateRequest",
			"content",
		)
		assertConstraintField(
			t,
			(AgentMemoryAddRequest{Scope: AgentMemoryScopeUser, Content: blank}).ValidateWire(),
			"AgentMemoryAddRequest",
			"content",
		)
	}
	assertConstraintField(
		t,
		(AgentMemoryItem{
			ID: agentMemoryWireItemID('1'), Scope: AgentMemoryScopeUser, Content: content,
			Origin: AgentMemoryOriginUser, Status: AgentMemoryStatusActive,
		}).ValidateWire(),
		"AgentMemoryItem",
		"content",
	)
}

func TestAgentMemoryItemIdentityWireConstraintIsExact(t *testing.T) {
	t.Parallel()

	validID := agentMemoryWireItemID('a')
	pinned := true
	valid := []struct {
		name     string
		shape    string
		validate func(string) error
	}{
		{name: "delete request", shape: "AgentMemoryItemRequest", validate: func(id string) error {
			return (AgentMemoryItemRequest{ID: id}).ValidateWire()
		}},
		{name: "review request", shape: "AgentMemoryReviewRequest", validate: func(id string) error {
			return (AgentMemoryReviewRequest{ID: id, Decision: AgentMemoryReviewApprove}).ValidateWire()
		}},
		{name: "update request", shape: "AgentMemoryUpdateRequest", validate: func(id string) error {
			return (AgentMemoryUpdateRequest{ID: id, Pinned: &pinned}).ValidateWire()
		}},
		{name: "item projection", shape: "AgentMemoryItem", validate: func(id string) error {
			return (AgentMemoryItem{
				ID: id, Scope: AgentMemoryScopeUser, Content: "fact",
				Origin: AgentMemoryOriginUser, Status: AgentMemoryStatusActive,
			}).ValidateWire()
		}},
	}
	for _, shape := range valid {
		if err := shape.validate(validID); err != nil {
			t.Errorf("%s rejected canonical identity: %v", shape.name, err)
		}
		for _, id := range []string{
			"mem_1",
			agentmemory.ItemIDPrefix + strings.Repeat("A", agentmemory.MaximumItemIDCharacters-len(agentmemory.ItemIDPrefix)),
			" " + validID,
		} {
			assertConstraintField(t, shape.validate(id), shape.shape, "id")
		}
	}
}

func TestAgentMemoryUpdateRequiresAtLeastOneChange(t *testing.T) {
	t.Parallel()

	id := agentMemoryWireItemID('1')
	content := "edited"
	pinned := true
	for _, request := range []AgentMemoryUpdateRequest{
		{ID: id, Content: &content},
		{ID: id, Pinned: &pinned},
		{ID: id, Content: &content, Pinned: &pinned},
	} {
		if err := request.ValidateWire(); err != nil {
			t.Errorf("ValidateWire rejected update with changes: %v", err)
		}
	}
	assertConstraintField(t, (AgentMemoryUpdateRequest{ID: id}).ValidateWire(), "AgentMemoryUpdateRequest", "content|pinned")
}

func TestUserAuthoredAgentMemoryCanOnlyBeActive(t *testing.T) {
	t.Parallel()

	base := AgentMemoryItem{
		ID: agentMemoryWireItemID('1'), Scope: AgentMemoryScopeUser, Content: "fact",
		Origin: AgentMemoryOriginUser, Status: AgentMemoryStatusActive,
	}
	if err := base.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected active user memory: %v", err)
	}
	pending := base
	pending.Status = AgentMemoryStatusPending
	assertConstraintField(t, pending.ValidateWire(), "AgentMemoryItem", "status")
	pending.Origin = AgentMemoryOriginAuto
	if err := pending.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected pending automatic memory: %v", err)
	}
}

func TestOutputCollectionWireConstraints(t *testing.T) {
	t.Parallel()

	pending := PendingInterruptSet{Interrupts: []Interrupt{}}
	assertConstraintField(t, pending.ValidateWire(), "PendingInterruptSet", "interrupts")

	capability := ProblemData{
		Type:                 ErrCapabilityNotNeg.Error(),
		RequiredCapabilities: []CapabilityRequirement{},
	}
	assertConstraintField(t, capability.ValidateWire(), "ProblemData", "requiredCapabilities")

	duplicate := CapabilityRequirement{Type: RequirementFeature, Name: "subagents"}
	capability.RequiredCapabilities = []CapabilityRequirement{duplicate, duplicate}
	assertConstraintField(t, capability.ValidateWire(), "ProblemData", "requiredCapabilities")
}

func TestQuestionWireConstraints(t *testing.T) {
	t.Parallel()

	valid := Question{Fields: []QuestionField{{
		Type:    QuestionFieldChoice,
		Prompt:  "Choose",
		Header:  "选择一下",
		Options: []QuestionOption{{Label: "A"}, {Label: "B"}},
	}}}
	if err := ValidateWireTree(valid); err != nil {
		t.Fatalf("ValidateWireTree rejected a valid question: %v", err)
	}

	oneOption := valid
	oneOption.Fields = slices.Clone(valid.Fields)
	oneOption.Fields[0].Options = []QuestionOption{{Label: "A"}}
	assertConstraintField(t, ValidateWireTree(oneOption), "Question", "fields[0].options")

	tooManyOptions := valid
	tooManyOptions.Fields = slices.Clone(valid.Fields)
	tooManyOptions.Fields[0].Options = []QuestionOption{{Label: "A"}, {Label: "B"}, {Label: "C"}, {Label: "D"}, {Label: "E"}}
	assertConstraintField(t, ValidateWireTree(tooManyOptions), "Question", "fields[0].options")

	blankPrompt := valid
	blankPrompt.Fields = slices.Clone(valid.Fields)
	blankPrompt.Fields[0].Prompt = " \t "
	assertConstraintField(t, ValidateWireTree(blankPrompt), "Question", "fields[0].prompt")

	blankOption := valid
	blankOption.Fields = slices.Clone(valid.Fields)
	blankOption.Fields[0].Options = slices.Clone(valid.Fields[0].Options)
	blankOption.Fields[0].Options[0].Label = "\n"
	assertConstraintField(t, ValidateWireTree(blankOption), "Question", "fields[0].options[0].label")

	tooManyFields := Question{Fields: make([]QuestionField, transcript.MaximumQuestionFields+1)}
	for index := range tooManyFields.Fields {
		tooManyFields.Fields[index] = QuestionField{Type: QuestionFieldText, Prompt: "Answer"}
	}
	assertConstraintField(t, ValidateWireTree(tooManyFields), "Question", "fields")

	longHeader := valid
	longHeader.Fields = slices.Clone(valid.Fields)
	longHeader.Fields[0].Header = "一二三四五六七八九十一二三"
	assertConstraintField(t, ValidateWireTree(longHeader), "Question", "fields[0].header")
}

func TestMCPSecretMapChangesRejectEmptyReplacement(t *testing.T) {
	t.Parallel()

	headers := MCPHeadersChange{Type: MCPSecretSet, Value: map[string]string{}}
	assertConstraintField(t, headers.ValidateWire(), "MCPHeadersChange", "value")

	environment := MCPEnvironmentChange{Type: MCPSecretSet, Value: map[string]string{}}
	assertConstraintField(t, environment.ValidateWire(), "MCPEnvironmentChange", "value")

	headers.Value = map[string]string{"X-API-Key": "secret"}
	if err := headers.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected a non-empty headers replacement: %v", err)
	}
}

func TestUpdateScheduleWorkspaceModesAreUnambiguous(t *testing.T) {
	t.Parallel()

	valid := []UpdateScheduleRequest{
		{ID: "sch_1", ExpectedRevision: 1},
		{ID: "sch_1", ExpectedRevision: 1, Workspace: &WorkspaceRef{Path: "/workspace"}},
		{ID: "sch_1", ExpectedRevision: 1, WorkspaceMode: ScheduleWorkspaceDefault},
	}
	for _, request := range valid {
		if err := ValidateWireTree(request); err != nil {
			t.Errorf("ValidateWireTree rejected legal schedule workspace patch %+v: %v", request, err)
		}
	}

	assertConstraintField(t, ValidateWireTree(UpdateScheduleRequest{
		ID:               "sch_1",
		ExpectedRevision: 1,
		Workspace:        &WorkspaceRef{Path: "/workspace"},
		WorkspaceMode:    ScheduleWorkspaceDefault,
	}), "UpdateScheduleRequest", "workspace")
	assertConstraintField(t, (UpdateScheduleRequest{
		ID:               "sch_1",
		ExpectedRevision: 1,
		WorkspaceMode:    "unknown",
	}).ValidateWire(), "UpdateScheduleRequest", "workspaceMode")
}

func TestScheduleInstructionsMustContainNonWhitespace(t *testing.T) {
	t.Parallel()

	blank := " \n\t"
	assertConstraintField(t, ValidateWireTree(CreateScheduleRequest{
		Instructions: blank,
		Cron:         "@daily",
	}), "CreateScheduleRequest", "instructions")
	assertConstraintField(t, ValidateWireTree(UpdateScheduleRequest{
		ID:               "sch_1",
		ExpectedRevision: 1,
		Instructions:     &blank,
	}), "UpdateScheduleRequest", "instructions")
	assertConstraintField(t, ValidateWireTree(Schedule{
		ID:           "sch_1",
		Instructions: blank,
		Cron:         "@daily",
		CreatedAt:    time.Unix(1, 0).UTC(),
		Revision:     1,
	}), "Schedule", "instructions")
}

func TestScheduleRequiresCron(t *testing.T) {
	t.Parallel()

	valid := Schedule{
		ID: "sch_1", Instructions: "run",
		Cron: "@daily", CreatedAt: time.Unix(1, 0).UTC(), Revision: 1,
	}
	if err := valid.ValidateWire(); err != nil {
		t.Fatalf("valid Schedule: %v", err)
	}
	valid.Cron = ""
	assertConstraintField(t, valid.ValidateWire(), "Schedule", "cron")
}

func TestApprovalRuleWireConstraintsCloseScopeShape(t *testing.T) {
	t.Parallel()
	valid := ApprovalRule{
		ID: "rule_1", Scope: ApprovalRuleScopeGlobal, Tool: "shell",
		Decision: ApprovalRuleDecisionAllow,
	}
	if err := valid.ValidateWire(); err != nil {
		t.Fatalf("valid global ApprovalRule: %v", err)
	}
	project := valid
	project.Scope, project.Dir = ApprovalRuleScopeProject, "/workspace"
	if err := project.ValidateWire(); err != nil {
		t.Fatalf("valid project ApprovalRule: %v", err)
	}

	tests := []struct {
		name  string
		field string
		value ApprovalRule
	}{
		{name: "missing id", field: "id", value: func() ApprovalRule { value := valid; value.ID = ""; return value }()},
		{name: "blank tool", field: "tool", value: func() ApprovalRule { value := valid; value.Tool = " \t"; return value }()},
		{name: "project without directory", field: "dir", value: func() ApprovalRule { value := valid; value.Scope = ApprovalRuleScopeProject; return value }()},
		{name: "session with directory", field: "dir", value: func() ApprovalRule {
			value := valid
			value.Scope, value.Dir = ApprovalRuleScopeSession, "/workspace"
			return value
		}()},
		{name: "global with directory", field: "dir", value: func() ApprovalRule { value := valid; value.Dir = "/workspace"; return value }()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertConstraintField(t, test.value.ValidateWire(), "ApprovalRule", test.field)
		})
	}
}

func TestMCPWireUnionsAcceptEveryLegalBranch(t *testing.T) {
	t.Parallel()

	seconds := 30
	valid := []WireValidator{
		MCPHandshakeTimeout{Type: MCPHandshakeUnbounded},
		MCPHandshakeTimeout{Type: MCPHandshakeBounded, Seconds: &seconds},
		MCPConnection{Type: MCPTransportStreamableHTTP, URL: "https://example.com/mcp"},
		MCPConnection{Type: MCPTransportStdio, Command: "mcp-server"},
		MCPConnectionInput{Type: MCPTransportStreamableHTTP, URL: "https://example.com/mcp"},
		MCPConnectionInput{Type: MCPTransportStdio, Command: "mcp-server"},
		MCPAuthorizationChange{Type: MCPSecretSet, Value: "Bearer secret"},
		MCPAuthorizationChange{Type: MCPSecretClear},
		MCPHeadersChange{Type: MCPSecretClear},
		MCPEnvironmentChange{Type: MCPSecretClear},
	}
	for _, value := range valid {
		if err := value.ValidateWire(); err != nil {
			t.Errorf("ValidateWire rejected legal %T branch: %v", value, err)
		}
	}

	stdioCandidate := MCPServerCandidate{
		Name: "filesystem", Enabled: false,
		Connection:       MCPConnectionInput{Type: MCPTransportStdio, Command: "mcp-server"},
		HandshakeTimeout: MCPHandshakeTimeout{Type: MCPHandshakeUnbounded},
	}
	if err := ValidateWireTree(stdioCandidate); err != nil {
		t.Fatalf("ValidateWireTree rejected a legal stdio candidate: %v", err)
	}

	assertConstraintField(t,
		(MCPConnectionInput{Type: MCPTransportStdio}).ValidateWire(),
		"MCPConnectionInput", "command",
	)
	assertConstraintField(t,
		(MCPConnectionInput{Type: MCPTransportStreamableHTTP}).ValidateWire(),
		"MCPConnectionInput", "url",
	)
	assertConstraintField(t,
		(MCPAuthorizationChange{Type: MCPSecretSet}).ValidateWire(),
		"MCPAuthorizationChange", "value",
	)
}

func TestMCPAuthorizationAttemptIdentityUsesCanonicalWireGrammar(t *testing.T) {
	request := MCPAuthorizationAttemptRequest{AttemptID: testsupport.MCPAuthorizationAttemptID}
	if err := request.ValidateWire(); err != nil {
		t.Fatalf("canonical request identity: %v", err)
	}
	request.AttemptID = "mcpauth_missing"
	assertConstraintField(t, request.ValidateWire(), "MCPAuthorizationAttemptRequest", "attemptId")

	attempt := MCPAuthorizationAttempt{ID: testsupport.MCPAuthorizationAttemptID, Server: "github"}
	if err := attempt.ValidateWire(); err != nil {
		t.Fatalf("canonical response identity: %v", err)
	}
	attempt.ID = "mcpauth_missing"
	assertConstraintField(t, attempt.ValidateWire(), "MCPAuthorizationAttempt", "id")
	attempt.ID = testsupport.MCPAuthorizationAttemptID
	attempt.Server = "GitHub"
	assertConstraintField(t, attempt.ValidateWire(), "MCPAuthorizationAttempt", "server")
}

func TestMCPServerIdentityUsesCanonicalWireGrammar(t *testing.T) {
	maximum := strings.Repeat("a", mcpserver.MaximumServerNameCharacters)
	valid := []WireValidator{
		MCPServerRequest{Server: maximum},
		CreateMCPAuthorizationAttemptRequest{Server: maximum},
		MCPListToolsRequest{Server: maximum},
		MCPServerCandidate{Name: maximum},
		UpdateMCPServerRequest{Server: maximum},
		MCPServer{Name: maximum},
		MCPTool{Server: maximum, Name: "read"},
	}
	for _, value := range valid {
		if err := value.ValidateWire(); err != nil {
			t.Errorf("ValidateWire rejected canonical %T identity: %v", value, err)
		}
	}
	if err := (MCPListToolsRequest{}).ValidateWire(); err != nil {
		t.Fatalf("optional all-server filter rejected omission: %v", err)
	}

	invalid := strings.Repeat("a", mcpserver.MaximumServerNameCharacters+1)
	tests := []struct {
		shape string
		field string
		err   error
	}{
		{"MCPServerRequest", "server", (MCPServerRequest{Server: invalid}).ValidateWire()},
		{"CreateMCPAuthorizationAttemptRequest", "server", (CreateMCPAuthorizationAttemptRequest{Server: "GitHub"}).ValidateWire()},
		{"MCPListToolsRequest", "server", (MCPListToolsRequest{Server: "with space"}).ValidateWire()},
		{"MCPServerCandidate", "name", (MCPServerCandidate{Name: "server/name"}).ValidateWire()},
		{"UpdateMCPServerRequest", "server", (UpdateMCPServerRequest{Server: invalid}).ValidateWire()},
		{"MCPServer", "name", (MCPServer{Name: "UPPER"}).ValidateWire()},
		{"MCPTool", "server", (MCPTool{Server: " server"}).ValidateWire()},
	}
	for _, test := range tests {
		assertConstraintField(t, test.err, test.shape, test.field)
	}
}

func TestMCPRemoteToolIdentityUsesCanonicalWireGrammar(t *testing.T) {
	maximum := strings.Repeat("a", mcpserver.MaximumRemoteToolNameCharacters)
	valid := []WireValidator{
		MCPServerCandidate{Name: "files", DisabledTools: []string{maximum}},
		MCPServer{Name: "files", AutoApproveTools: []string{maximum}},
		MCPTool{Server: "files", Name: maximum},
	}
	for _, value := range valid {
		if err := value.ValidateWire(); err != nil {
			t.Errorf("ValidateWire rejected canonical %T remote tool identity: %v", value, err)
		}
	}

	invalidUpdate := []string{"tool/name"}
	tests := []struct {
		shape string
		field string
		err   error
	}{
		{"MCPServerCandidate", "disabledTools[0]", (MCPServerCandidate{Name: "files", DisabledTools: []string{"with space"}}).ValidateWire()},
		{"UpdateMCPServerRequest", "autoApproveTools[0]", (UpdateMCPServerRequest{Server: "files", AutoApproveTools: &invalidUpdate}).ValidateWire()},
		{"MCPServer", "disabledTools[0]", (MCPServer{Name: "files", DisabledTools: []string{"工具"}}).ValidateWire()},
		{"MCPTool", "name", (MCPTool{Server: "files", Name: strings.Repeat("a", mcpserver.MaximumRemoteToolNameCharacters+1)}).ValidateWire()},
	}
	for _, test := range tests {
		assertConstraintField(t, test.err, test.shape, test.field)
	}

	tooMany := make([]string, mcpserver.MaxRemoteToolsPerServer+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("tool_%d", i)
	}
	assertConstraintField(
		t,
		(MCPServerCandidate{Name: "files", DisabledTools: tooMany}).ValidateWire(),
		"MCPServerCandidate",
		"disabledTools",
	)
}

func TestProblemDataWireUnion(t *testing.T) {
	t.Parallel()

	validCapability := []CapabilityRequirement{{Type: RequirementFeature, Name: "subagents"}}
	tests := []struct {
		name    string
		problem ProblemData
		field   string
	}{
		{name: "ordinary first party problem", problem: ProblemData{Type: ProblemRunLost}},
		{
			name:    "inline status carries no server authored copy",
			problem: ProblemData{Type: ProblemMCPDialFailed, Detail: "connection failed"},
			field:   "detail",
		},
		{
			name: "capability problem carries its gaps",
			problem: ProblemData{
				Type: ErrCapabilityNotNeg.Error(), RequiredCapabilities: validCapability,
			},
		},
		{
			name:    "capability problem requires gaps",
			problem: ProblemData{Type: ErrCapabilityNotNeg.Error()},
			field:   "requiredCapabilities",
		},
		{
			name:    "structured fields belong to their variant",
			problem: ProblemData{Type: ProblemRunLost, RequiredCapabilities: validCapability},
			field:   "requiredCapabilities",
		},
		{
			name: "active run belongs only to the conflict",
			problem: ProblemData{
				Type:      ProblemRunLost,
				ActiveRun: &ActiveRunRef{RunID: "run_1", Status: RunStatusRunning},
			},
			field: "activeRun",
		},
		{
			name:    "idempotency progress requires a delay",
			problem: ProblemData{Type: ErrIdempotencyInProgress.Error()},
			field:   "retryAfterSeconds",
		},
		{
			name:    "retry delay is positive",
			problem: ProblemData{Type: ProblemTimeout, RetryAfterSeconds: -1},
			field:   "retryAfterSeconds",
		},
		{
			name: "plugin problem uses its namespace",
			problem: ProblemData{
				Type: "plugin:acme/model_timeout", Detail: "try another region",
				RetryAfterSeconds: 2,
			},
		},
		{
			name: "plugin problem cannot borrow first party fields",
			problem: ProblemData{
				Type: "plugin:acme/model_timeout", RequiredCapabilities: validCapability,
			},
			field: "requiredCapabilities",
		},
		{
			name:    "unnamespaced extension is rejected",
			problem: ProblemData{Type: "model_timeout"},
			field:   "type",
		},
		{
			name:    "malformed plugin namespace is rejected",
			problem: ProblemData{Type: "plugin:Acme/model_timeout"},
			field:   "type",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateWireTree(test.problem)
			if test.field == "" {
				if err != nil {
					t.Fatalf("ValidateWireTree rejected a valid problem: %v", err)
				}
				return
			}
			assertConstraintField(t, err, "ProblemData", test.field)
		})
	}
	if int64(math.MaxInt) > MaximumDurationSeconds {
		tooLongSeconds64 := MaximumDurationSeconds + 1
		tooLongSeconds := int(tooLongSeconds64)
		assertConstraintField(t, ValidateWireTree(ProblemData{
			Type: ProblemTimeout, RetryAfterSeconds: tooLongSeconds,
		}), "ProblemData", "retryAfterSeconds")
	}
}

func TestProblemDataStructuredLeavesAreValidated(t *testing.T) {
	t.Parallel()

	activeRun := ProblemData{
		Type:      ErrSessionHasActiveRun.Error(),
		ActiveRun: &ActiveRunRef{Status: RunStatus("teleported")},
	}
	err := ValidateWireTree(activeRun)
	assertConstraintField(t, err, "ProblemData", "activeRun.runId")
	assertConstraintField(t, err, "ProblemData", "activeRun.status")

	invalidFields := ProblemData{
		Type:   ErrInvalidParams.Error(),
		Errors: []FieldError{{}},
	}
	err = ValidateWireTree(invalidFields)
	assertConstraintField(t, err, "ProblemData", "errors[0].field")
	assertConstraintField(t, err, "ProblemData", "errors[0].detail")
}

func TestRunOpeningResponseWireConstraints(t *testing.T) {
	t.Parallel()

	start := StartRunResponse{RunID: "run_1", SegmentID: "seg_1"}
	assertConstraintField(t, start.ValidateWire(), "StartRunResponse", "userItemId")
	start.UserItemID = "item_1"
	if err := start.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected a complete start response: %v", err)
	}

	resume := ResumeRunResponse{RunID: "run_1", SegmentID: "seg_2"}
	if err := resume.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected a response-only resume: %v", err)
	}
	empty := ""
	resume.UserItemID = &empty
	assertConstraintField(t, resume.ValidateWire(), "ResumeRunResponse", "userItemId")
}

func TestCancelRunReasonWireConstraintUsesUnicodeCharacters(t *testing.T) {
	t.Parallel()

	const maximum = 1024
	valid := CancelRunRequest{RunID: "run_1", Reason: strings.Repeat("界", maximum)}
	if err := valid.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected the cancellation reason boundary: %v", err)
	}

	valid.Reason += "界"
	assertConstraintField(t, valid.ValidateWire(), "CancelRunRequest", "reason")
}

func TestRunProtocolProfileWireConstraints(t *testing.T) {
	t.Parallel()

	valid := RunProtocolProfile{
		RequiredFeatures: []RunProtocolFeature{RunProtocolFeatureSubagents},
		InterruptTypes:   []InterruptType{InterruptApproval, InterruptQuestion},
	}
	if err := valid.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected a valid profile: %v", err)
	}

	repeatedFeature := valid
	repeatedFeature.RequiredFeatures = append(repeatedFeature.RequiredFeatures, RunProtocolFeatureSubagents)
	assertConstraintField(t, repeatedFeature.ValidateWire(), "RunProtocolProfile", "requiredFeatures")

	repeatedInterrupt := valid
	repeatedInterrupt.InterruptTypes = append(repeatedInterrupt.InterruptTypes, InterruptApproval)
	assertConstraintField(t, repeatedInterrupt.ValidateWire(), "RunProtocolProfile", "interruptTypes")

	unknown := valid
	unknown.RequiredFeatures = []RunProtocolFeature{"telepathy"}
	assertConstraintField(t, unknown.ValidateWire(), "RunProtocolProfile", "requiredFeatures[0]")
}

func TestValidateWireTreeComposesNestedConstraints(t *testing.T) {
	t.Parallel()

	pending := PendingInterruptSet{
		RootRunID: "run_root",
		SessionID: "ses_1",
		CreatedAt: time.Date(2026, 7, 30, 1, 0, 0, 0, time.UTC),
		Interrupts: []Interrupt{{
			ItemID: "item_question",
			Type:   InterruptQuestion,
			Payload: &InterruptPayload{
				Question: &Question{Fields: []QuestionField{{
					Prompt: "Continue?", Type: QuestionFieldText,
				}}},
			},
		}},
	}
	assertConstraintField(t, pending.Interrupts[0].ValidateWire(), "Interrupt", "runId")
	assertConstraintField(
		t,
		ValidateWireTree(pending),
		"PendingInterruptSet",
		"interrupts[0].runId",
	)
}

func TestTextContentMustContainNonWhitespace(t *testing.T) {
	t.Parallel()

	blank := ContentBlock{
		Type: ContentBlockText,
		Text: " \n\t",
	}
	assertConstraintField(t, blank.ValidateWire(), "ContentBlock", "text")
	assertConstraintField(t, ValidateWireTree(StartRunRequest{
		SessionID: "ses_1",
		Input:     []ContentBlock{blank},
	}), "StartRunRequest", "input[0].text")
	if err := (ContentBlock{Type: ContentBlockText, Text: " continue "}).ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected meaningful text: %v", err)
	}
	if err := (ContentBlock{Type: ContentBlockImage, Mime: "image/png", Data: "AA=="}).ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected image-only content: %v", err)
	}
}

func TestItemTimingVocabularyIsVariantExclusive(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	message := Item{
		ID: "item_message", RunID: "run_1", Status: ItemStatusCompleted,
		Type: ItemTypeUserMessage, CreatedAt: at,
		Content: []ContentBlock{{Type: ContentBlockText, Text: "hello"}},
	}
	if err := message.ValidateWire(); err != nil {
		t.Fatalf("message timing: %v", err)
	}
	message.StartedAt = at
	assertConstraintField(t, message.ValidateWire(), "Item", "startedAt")

	toolCall := Item{
		ID: "item_tool", RunID: "run_1", Status: ItemStatusRunning,
		Type: ItemTypeToolCall, StartedAt: at,
		Tool: &ToolInvocation{Name: "shell", Arguments: map[string]any{"command": "pwd"}},
	}
	if err := toolCall.ValidateWire(); err != nil {
		t.Fatalf("tool-call timing: %v", err)
	}
	toolCall.CreatedAt = at
	assertConstraintField(t, toolCall.ValidateWire(), "Item", "createdAt")

	finishedAt := at.Add(time.Minute)
	toolCall = Item{
		ID: "item_tool", RunID: "run_1", Status: ItemStatusIncomplete,
		Type: ItemTypeToolCall, StartedAt: at, FinishedAt: finishedAt,
		Tool: &ToolInvocation{Name: "shell", Arguments: map[string]any{"command": "pwd"}},
	}
	if err := toolCall.ValidateWire(); err != nil {
		t.Fatalf("terminal tool-call with unknown execution duration: %v", err)
	}
	durationMillis := int64(500)
	toolCall.DurationMillis = &durationMillis
	if err := toolCall.ValidateWire(); err != nil {
		t.Fatalf("terminal tool-call with exact execution duration: %v", err)
	}
	toolCall.Status = ItemStatusCompleted
	toolCall.DurationMillis = nil
	assertConstraintField(t, toolCall.ValidateWire(), "Item", "durationMillis")
	toolCall.DurationMillis = &durationMillis
	if err := toolCall.ValidateWire(); err != nil {
		t.Fatalf("completed tool-call with exact execution duration: %v", err)
	}

	artifactToolCall := ArtifactItem{
		ID: "item_tool", RunID: "run_1", Status: ItemStatusIncomplete,
		Type: ItemTypeToolCall, StartedAt: at, FinishedAt: finishedAt,
		Tool: &ArtifactToolInvocation{Name: "shell", Arguments: map[string]any{"command": "pwd"}},
	}
	if err := artifactToolCall.ValidateWire(); err != nil {
		t.Fatalf("artifact tool-call timing: %v", err)
	}
	artifactToolCall.Status = ItemStatusCompleted
	assertConstraintField(t, artifactToolCall.ValidateWire(), "ArtifactItem", "durationMillis")
	artifactToolCall.DurationMillis = &durationMillis
	if err := artifactToolCall.ValidateWire(); err != nil {
		t.Fatalf("completed artifact tool-call with exact execution duration: %v", err)
	}
	artifactToolCall.CreatedAt = at
	assertConstraintField(t, artifactToolCall.ValidateWire(), "ArtifactItem", "createdAt")
}

func TestItemPayloadsMatchTheirLifecycleFacts(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name  string
		shape string
		field string
		value WireValidator
	}{
		{
			name:  "runtime user message owns content",
			shape: "Item", field: "content",
			value: Item{ID: "item_user", RunID: "run_1", Status: ItemStatusCompleted, Type: ItemTypeUserMessage, CreatedAt: at},
		}, {
			name:  "runtime terminal agent message owns phase",
			shape: "Item", field: "phase",
			value: Item{ID: "item_agent", RunID: "run_1", Status: ItemStatusCompleted, Type: ItemTypeAgentMessage, CreatedAt: at},
		}, {
			name:  "runtime terminal reasoning owns text",
			shape: "Item", field: "text",
			value: Item{ID: "item_reasoning", RunID: "run_1", Status: ItemStatusCompleted, Type: ItemTypeReasoning, CreatedAt: at},
		}, {
			name:  "runtime question owns its form",
			shape: "Item", field: "question",
			value: Item{ID: "item_question", RunID: "run_1", Status: ItemStatusCompleted, Type: ItemTypeQuestion, CreatedAt: at},
		}, {
			name:  "runtime tool call owns its invocation",
			shape: "Item", field: "tool",
			value: Item{ID: "item_tool", RunID: "run_1", Status: ItemStatusRunning, Type: ItemTypeToolCall, StartedAt: at},
		}, {
			name:  "runtime compaction owns its summary",
			shape: "Item", field: "summary",
			value: Item{ID: "item_compaction", RunID: "run_1", Status: ItemStatusCompleted, Type: ItemTypeCompaction, CreatedAt: at},
		}, {
			name:  "artifact user message owns content",
			shape: "ArtifactItem", field: "content",
			value: ArtifactItem{ID: "item_user", RunID: "run_1", Status: ItemStatusCompleted, Type: ItemTypeUserMessage, CreatedAt: at},
		}, {
			name:  "artifact agent message owns phase",
			shape: "ArtifactItem", field: "phase",
			value: ArtifactItem{ID: "item_agent", RunID: "run_1", Status: ItemStatusCompleted, Type: ItemTypeAgentMessage, CreatedAt: at},
		}, {
			name:  "artifact reasoning owns text",
			shape: "ArtifactItem", field: "text",
			value: ArtifactItem{ID: "item_reasoning", RunID: "run_1", Status: ItemStatusCompleted, Type: ItemTypeReasoning, CreatedAt: at},
		}, {
			name:  "artifact question owns its form",
			shape: "ArtifactItem", field: "question",
			value: ArtifactItem{ID: "item_question", RunID: "run_1", Status: ItemStatusCompleted, Type: ItemTypeQuestion, CreatedAt: at},
		}, {
			name:  "artifact tool call owns its invocation",
			shape: "ArtifactItem", field: "tool",
			value: ArtifactItem{ID: "item_tool", RunID: "run_1", Status: ItemStatusRunning, Type: ItemTypeToolCall, StartedAt: at},
		}, {
			name:  "artifact compaction owns its summary",
			shape: "ArtifactItem", field: "summary",
			value: ArtifactItem{ID: "item_compaction", RunID: "run_1", Status: ItemStatusCompleted, Type: ItemTypeCompaction, CreatedAt: at},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertConstraintField(t, test.value.ValidateWire(), test.shape, test.field)
		})
	}
}

func TestItemStatusesMatchTheirLifecycleOwners(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 8, 29, 11, 0, 0, 0, time.UTC)
	content := []ContentBlock{{Type: ContentBlockText, Text: "hello"}}
	for _, test := range []struct {
		name  string
		shape string
		field string
		value WireValidator
	}{
		{
			name:  "user message has no running lifecycle",
			shape: "Item", field: "status",
			value: Item{ID: "item_user", RunID: "run_1", Status: ItemStatusRunning, Type: ItemTypeUserMessage, CreatedAt: at, Content: content},
		}, {
			name:  "agent message has no incomplete durable state",
			shape: "Item", field: "status",
			value: Item{ID: "item_agent", RunID: "run_1", Status: ItemStatusIncomplete, Type: ItemTypeAgentMessage, CreatedAt: at, Phase: MessagePhaseCommentary, Content: content},
		}, {
			name:  "reasoning has no incomplete durable state",
			shape: "Item", field: "status",
			value: Item{ID: "item_reasoning", RunID: "run_1", Status: ItemStatusIncomplete, Type: ItemTypeReasoning, CreatedAt: at, Text: "thinking"},
		}, {
			name:  "question is a complete prompt fact",
			shape: "Item", field: "status",
			value: Item{ID: "item_question", RunID: "run_1", Status: ItemStatusRunning, Type: ItemTypeQuestion, CreatedAt: at, Question: &Question{Fields: []QuestionField{{Type: QuestionFieldText, Prompt: "Continue?"}}}},
		}, {
			name:  "compaction is a complete boundary",
			shape: "Item", field: "status",
			value: Item{ID: "item_compaction", RunID: "run_1", Status: ItemStatusRunning, Type: ItemTypeCompaction, CreatedAt: at, Summary: "Earlier work"},
		}, {
			name:  "portable artifact has no running tool",
			shape: "ArtifactItem", field: "status",
			value: ArtifactItem{ID: "item_tool", RunID: "run_1", Status: ItemStatusRunning, Type: ItemTypeToolCall, StartedAt: at, Tool: &ArtifactToolInvocation{Name: "shell", Arguments: map[string]any{"command": "pwd"}}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertConstraintField(t, test.value.ValidateWire(), test.shape, test.field)
		})
	}

	startedUser := StreamEvent{
		Type: StreamItemStarted,
		Item: &Item{ID: "item_user", RunID: "run_1", Status: ItemStatusCompleted, Type: ItemTypeUserMessage, CreatedAt: at, Content: content},
	}
	assertConstraintField(t, startedUser.ValidateWire(), "StreamEvent", "item.type")

	completedAnchor := StreamEvent{
		Type: StreamItemCompleted,
		Item: &Item{ID: "item_agent", RunID: "run_1", Status: ItemStatusRunning, Type: ItemTypeAgentMessage, CreatedAt: at},
	}
	assertConstraintField(t, completedAnchor.ValidateWire(), "StreamEvent", "item.status")
}

func TestPublishedLimitWireConstraints(t *testing.T) {
	t.Parallel()

	replay := RunReplayLimits{Scope: ReplayScopeRuntimeInstanceRootSegment}
	assertConstraintField(t, replay.ValidateWire(), "RunReplayLimits", "maxEvents")

	subscription := SubscriptionLimits{}
	err := subscription.ValidateWire()
	assertConstraintField(t, err, "SubscriptionLimits", "maxTopics")
	assertConstraintField(t, err, "SubscriptionLimits", "maxWatches")

	validRuntimeLimits := func(maxConcurrentRuns *int) RuntimeLimits {
		return RuntimeLimits{
			MaxConcurrentRuns: maxConcurrentRuns,
			Idempotency:       IdempotencyLimits{RetentionSeconds: 1, Namespace: testsupport.IdempotencyNamespace},
			RunReplay: RunReplayLimits{
				Scope: ReplayScopeRuntimeInstanceRootSegment, MaxEvents: 1, MaxBytes: 1,
			},
			MCPAuthorizationAttempts: MCPAuthorizationAttemptLimits{RetentionSeconds: 1},
			RuntimeSubscription:      SubscriptionLimits{MaxTopics: 1, MaxWatches: 1},
		}
	}
	if err := validRuntimeLimits(nil).ValidateWire(); err != nil {
		t.Fatalf("unbounded Runtime limits: %v", err)
	}
	positiveConcurrentRuns := 1
	if err := validRuntimeLimits(&positiveConcurrentRuns).ValidateWire(); err != nil {
		t.Fatalf("bounded Runtime limits: %v", err)
	}
	invalidNamespace := validRuntimeLimits(nil)
	invalidNamespace.Idempotency.Namespace = "idp_test"
	assertConstraintField(t, invalidNamespace.Idempotency.ValidateWire(), "IdempotencyLimits", "namespace")
	zeroConcurrentRuns := 0
	assertConstraintField(t, validRuntimeLimits(&zeroConcurrentRuns).ValidateWire(), "RuntimeLimits", "maxConcurrentRuns")
	if int64(math.MaxInt) > MaximumDurationSeconds {
		tooLongSeconds64 := MaximumDurationSeconds + 1
		tooLongSeconds := int(tooLongSeconds64)
		tooLong := validRuntimeLimits(nil)
		tooLong.Idempotency.RetentionSeconds = tooLongSeconds
		tooLong.MCPAuthorizationAttempts.RetentionSeconds = tooLongSeconds
		assertConstraintField(t, ValidateWireTree(tooLong), "RuntimeLimits", "idempotency.retentionSeconds")
		assertConstraintField(t, ValidateWireTree(tooLong), "RuntimeLimits", "mcpAuthorizationAttempts.retentionSeconds")
		assertConstraintField(t, (MCPHandshakeTimeout{
			Type: MCPHandshakeBounded, Seconds: &tooLongSeconds,
		}).ValidateWire(), "MCPHandshakeTimeout", "seconds")
	}

	negativeTokens, negativeSteps, negativeBudget := int64(-1), -1, -0.01
	start := StartRunRequest{SessionID: "ses_1", Input: []ContentBlock{{Type: ContentBlockText, Text: "go"}}, Limits: &RunLimits{MaxTotalTokens: &negativeTokens}}
	assertConstraintField(t, ValidateWireTree(start), "StartRunRequest", "limits.maxTotalTokens")

	run := RunLimits{MaxSteps: &negativeSteps}
	assertConstraintField(t, run.ValidateWire(), "RunLimits", "maxSteps")

	artifact := ArtifactRunLimits{MaxBudgetUSD: &negativeBudget}
	assertConstraintField(t, artifact.ValidateWire(), "ArtifactRunLimits", "maxBudgetUsd")

	assertConstraintField(t, (RunLimits{}).ValidateWire(), "RunLimits", "maxTotalTokens|maxSteps|maxBudgetUsd")
	zeroContext := int64(0)
	assertConstraintField(t, (ModelTokenLimits{}).ValidateWire(), "ModelTokenLimits", "contextWindow|maxInputTokens|maxOutputTokens")
	assertConstraintField(t, (ModelTokenLimits{ContextWindow: &zeroContext}).ValidateWire(), "ModelTokenLimits", "contextWindow")
}

func TestModelSelectionWireConstraintsRequireAnExactPair(t *testing.T) {
	t.Parallel()

	start := StartRunRequest{
		SessionID: "ses_1",
		Input:     []ContentBlock{{Type: ContentBlockText, Text: "go"}},
		Provider:  "provider",
	}
	assertConstraintField(t, start.ValidateWire(), "StartRunRequest", "model")

	provider := "provider"
	update := UpdateSessionRequest{
		SessionID: "ses_1", ExpectedRevision: 1, Provider: &provider,
	}
	assertConstraintField(t, update.ValidateWire(), "UpdateSessionRequest", "model")

	model := "model"
	update = UpdateSessionRequest{
		SessionID: "ses_1", ExpectedRevision: 1, Model: &model,
	}
	assertConstraintField(t, update.ValidateWire(), "UpdateSessionRequest", "provider")

	goalStart := StartGoalRequest{SessionID: "ses_1", Objective: "finish", Provider: "provider"}
	assertConstraintField(t, goalStart.ValidateWire(), "StartGoalRequest", "model")
	goalStart = StartGoalRequest{SessionID: "ses_1", Objective: " \t", ReasoningEffort: "high"}
	assertConstraintField(t, goalStart.ValidateWire(), "StartGoalRequest", "objective")
	goalStart.Objective = "finish"
	goalSelectionErr := goalStart.ValidateWire()
	assertConstraintField(t, goalSelectionErr, "StartGoalRequest", "provider")
	assertConstraintField(t, goalSelectionErr, "StartGoalRequest", "model")

	scheduleCreate := CreateScheduleRequest{Instructions: "run", Cron: "0 0 * * *", Model: "model"}
	assertConstraintField(t, scheduleCreate.ValidateWire(), "CreateScheduleRequest", "provider")

	schedule := Schedule{ID: "sch_1", Instructions: "run", Cron: "@daily", Revision: 1, Provider: "provider"}
	assertConstraintField(t, schedule.ValidateWire(), "Schedule", "model")
	schedule.Provider, schedule.Model = "", "model"
	assertConstraintField(t, schedule.ValidateWire(), "Schedule", "provider")
	schedule.Model, schedule.ReasoningEffort = "", "high"
	scheduleSelectionErr := schedule.ValidateWire()
	assertConstraintField(t, scheduleSelectionErr, "Schedule", "provider")
	assertConstraintField(t, scheduleSelectionErr, "Schedule", "model")
	scheduleUpdate := UpdateScheduleRequest{ID: "sch_1", ExpectedRevision: 1, Provider: &provider}
	assertConstraintField(t, scheduleUpdate.ValidateWire(), "UpdateScheduleRequest", "model")
}

func TestGoalWireConstraintsCloseLifecycleState(t *testing.T) {
	t.Parallel()

	valid := func(status GoalStatus, reason *GoalReason) Goal {
		return Goal{
			SessionID: "ses_1", Objective: "finish", Status: status, Reason: reason,
			Provider: "deepseek", Model: "deepseek-chat",
			CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(2, 0).UTC(),
		}
	}

	for _, test := range []struct {
		name  string
		field string
		value Goal
	}{
		{name: "missing objective", field: "objective", value: func() Goal { value := valid(GoalActive, nil); value.Objective = ""; return value }()},
		{name: "blank objective", field: "objective", value: func() Goal { value := valid(GoalActive, nil); value.Objective = " \n"; return value }()},
		{name: "missing provider", field: "provider", value: func() Goal { value := valid(GoalActive, nil); value.Provider = ""; return value }()},
		{name: "missing model", field: "model", value: func() Goal { value := valid(GoalActive, nil); value.Model = ""; return value }()},
		{name: "active reason", field: "reason", value: valid(GoalActive, &GoalReason{Code: GoalReasonStoppedByUser})},
		{name: "completing reason", field: "reason", value: valid(GoalCompleting, &GoalReason{Code: GoalReasonStoppedByUser})},
		{name: "paused without reason", field: "reason", value: valid(GoalPaused, nil)},
		{name: "paused with blocked reason", field: "reason.code", value: valid(GoalPaused, &GoalReason{Code: GoalReasonRunBudgetReached})},
		{name: "blocked without reason", field: "reason", value: valid(GoalBlocked, nil)},
		{name: "blocked with paused reason", field: "reason.code", value: valid(GoalBlocked, &GoalReason{Code: GoalReasonStoppedByUser})},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertConstraintField(t, ValidateWireTree(test.value), "Goal", test.field)
		})
	}

	for _, test := range []struct {
		name  string
		field string
		value GoalReason
	}{
		{name: "fixed reason with detail", field: "detail", value: GoalReason{Code: GoalReasonStoppedByUser, Detail: "extra"}},
		{name: "run outcome without detail", field: "detail", value: GoalReason{Code: GoalReasonRunNotCompleted}},
		{name: "model block without detail", field: "detail", value: GoalReason{Code: GoalReasonBlockedByModel}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertConstraintField(t, test.value.ValidateWire(), "GoalReason", test.field)
		})
	}

	if err := ValidateWireTree(valid(GoalPaused, &GoalReason{
		Code: GoalReasonRunNotCompleted, Detail: "failed",
	})); err != nil {
		t.Fatalf("valid paused Goal: %v", err)
	}
	if err := ValidateWireTree(valid(GoalBlocked, &GoalReason{
		Code: GoalReasonBlockedByModel, Detail: "needs user input",
	})); err != nil {
		t.Fatalf("valid blocked Goal: %v", err)
	}

	assertConstraintField(t, (UpdateGoalRequest{SessionID: "ses_1"}).ValidateWire(), "UpdateGoalRequest", "objective")
	assertConstraintField(t, (UpdateGoalRequest{SessionID: "ses_1", Objective: "\t"}).ValidateWire(), "UpdateGoalRequest", "objective")
}

func TestModelIdentitiesAreBoundedCanonicalWireValues(t *testing.T) {
	t.Parallel()

	valid := StartRunRequest{
		SessionID:       "ses_1",
		Input:           []ContentBlock{{Type: ContentBlockText, Text: "go"}},
		Provider:        strings.Repeat("提", modelref.MaximumProviderIdentityCharacters),
		Model:           strings.Repeat("模", modelref.MaximumModelIdentityCharacters),
		ReasoningEffort: strings.Repeat("强", modelref.MaximumReasoningEffortCharacters),
	}
	if err := valid.ValidateWire(); err != nil {
		t.Fatalf("identity boundaries: %v", err)
	}

	for _, test := range []struct {
		name  string
		shape string
		field string
		value WireValidator
	}{
		{
			name: "provider whitespace", shape: "StartRunRequest", field: "provider",
			value: StartRunRequest{SessionID: "ses_1", Input: []ContentBlock{{Type: ContentBlockText, Text: "go"}}, Provider: "open ai", Model: "gpt-5"},
		},
		{
			name: "model too long", shape: "StartRunRequest", field: "model",
			value: StartRunRequest{SessionID: "ses_1", Input: []ContentBlock{{Type: ContentBlockText, Text: "go"}}, Provider: "openai", Model: strings.Repeat("m", modelref.MaximumModelIdentityCharacters+1)},
		},
		{
			name: "reasoning control", shape: "StartRunRequest", field: "reasoningEffort",
			value: StartRunRequest{SessionID: "ses_1", Input: []ContentBlock{{Type: ContentBlockText, Text: "go"}}, Provider: "openai", Model: "gpt-5", ReasoningEffort: "high\t"},
		},
		{
			name: "provider result control", shape: "Provider", field: "id",
			value: Provider{ID: "openai\x00shadow", CredentialRequirement: ProviderAPIKeyRequired},
		},
		{
			name: "catalog model id too long", shape: "Model", field: "id",
			value: Model{ID: strings.Repeat("m", modelref.MaximumModelIdentityCharacters+1), Provider: "openai"},
		},
		{
			name: "reasoning level whitespace", shape: "ModelCapabilities", field: "reasoningLevels[0]",
			value: ModelCapabilities{Reasoning: true, ReasoningLevels: []string{"very high"}},
		},
		{
			name: "reasoning level too long", shape: "ModelCapabilities", field: "reasoningLevels[0]",
			value: ModelCapabilities{Reasoning: true, ReasoningLevels: []string{strings.Repeat("e", modelref.MaximumReasoningEffortCharacters+1)}},
		},
		{
			name: "usage model key whitespace", shape: "Usage", field: "byModel[\"bad model\"]",
			value: Usage{ByModel: map[string]ModelUsage{"bad model": {}}},
		},
		{
			name: "artifact model key too long", shape: "ArtifactUsage", field: "byModel[\"" + strings.Repeat("m", modelref.MaximumModelIdentityCharacters+1) + "\"]",
			value: ArtifactUsage{ByModel: map[string]ArtifactModelUsage{
				strings.Repeat("m", modelref.MaximumModelIdentityCharacters+1): {},
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertConstraintField(t, test.value.ValidateWire(), test.shape, test.field)
		})
	}
}

func TestUniqueItemsComparesJSONValues(t *testing.T) {
	t.Parallel()

	t.Run("dynamic objects", func(t *testing.T) {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("uniqueItems panicked for JSON objects: %v", recovered)
			}
		}()
		violation := uniqueItems("items", []any{
			map[string]any{"type": "feature", "name": "tool"},
			map[string]any{"name": "tool", "type": "feature"},
		})
		if violation.Field != "items" {
			t.Fatalf("uniqueItems violation = %+v, want repeated JSON object", violation)
		}
	})

	t.Run("distinct pointers to equal values", func(t *testing.T) {
		left, right := "same", "same"
		violation := uniqueItems("items", []*string{&left, &right})
		if violation.Field != "items" {
			t.Fatalf("uniqueItems violation = %+v, want repeated pointed-to value", violation)
		}
	})

	t.Run("distinct nested values", func(t *testing.T) {
		violation := uniqueItems("items", []any{
			map[string]any{"name": "one", "values": []any{1.0, true}},
			map[string]any{"name": "two", "values": []any{1.0, true}},
		})
		if violation.Field != "" {
			t.Fatalf("uniqueItems violation = %+v, want none", violation)
		}
	})
}

func TestRequestBoundsAreWireConstraints(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		shape string
		field string
		value WireValidator
	}{
		{shape: "PageQuery", field: "limit", value: PageQuery{Limit: wireIntPointer(-1)}},
		{shape: "PageQuery", field: "limit", value: PageQuery{Limit: wireIntPointer(0)}},
		{shape: "PageQuery", field: "cursor", value: PageQuery{Cursor: strings.Repeat("x", MaximumPaginationCursorCharacters+1)}},
		{shape: "ListSessionsRequest", field: "cursor", value: ListSessionsRequest{PageQuery: PageQuery{Cursor: strings.Repeat("x", MaximumPaginationCursorCharacters+1)}}},
		{shape: "ListSessionsRequest", field: "search", value: ListSessionsRequest{Search: strings.Repeat("x", 1025)}},
		{shape: "RunEvent", field: "eventId", value: RunEvent{RunID: "run_1", SegmentID: "seg_1", EventID: IDPrefixEvent + strings.Repeat("x", MaximumRunEventIDCharacters)}},
		{shape: "RunEvent", field: "eventId", value: RunEvent{RunID: "run_1", SegmentID: "seg_1", EventID: "opaque"}},
		{shape: "SubscribeRunResponse", field: "headEventId", value: SubscribeRunResponse{RunID: "run_1", SegmentID: "seg_1", HeadEventID: wireStringPointer(IDPrefixEvent + strings.Repeat("x", MaximumRunEventIDCharacters))}},
		{shape: "SubscribeRunResponse", field: "headEventId", value: SubscribeRunResponse{RunID: "run_1", SegmentID: "seg_1", HeadEventID: wireStringPointer("opaque")}},
		{shape: "GetDiffRequest", field: "limit", value: GetDiffRequest{Limit: wireIntPointer(-1)}},
		{shape: "GetDiffRequest", field: "limit", value: GetDiffRequest{Limit: wireIntPointer(0)}},
		{shape: "GetFileHeadRequest", field: "lines", value: GetFileHeadRequest{Path: "README.md", Lines: wireIntPointer(-1)}},
		{shape: "GetFileHeadRequest", field: "lines", value: GetFileHeadRequest{Path: "README.md", Lines: wireIntPointer(0)}},
		{shape: "GrepRequest", field: "limit", value: GrepRequest{Query: "needle", Limit: wireIntPointer(-1)}},
		{shape: "GrepRequest", field: "limit", value: GrepRequest{Query: "needle", Limit: wireIntPointer(0)}},
		{shape: "ReadFileRequest", field: "startLine", value: ReadFileRequest{Path: "README.md", StartLine: wireIntPointer(-1)}},
		{shape: "ReadFileRequest", field: "startLine", value: ReadFileRequest{Path: "README.md", StartLine: wireIntPointer(0)}},
		{shape: "ReadFileRequest", field: "endLine", value: ReadFileRequest{Path: "README.md", EndLine: wireIntPointer(-1)}},
		{shape: "ReadFileRequest", field: "endLine", value: ReadFileRequest{Path: "README.md", EndLine: wireIntPointer(0)}},
		{shape: "ReadFileRequest", field: "maxBytes", value: ReadFileRequest{Path: "README.md", MaxBytes: wireIntPointer(-1)}},
		{shape: "ReadFileRequest", field: "maxBytes", value: ReadFileRequest{Path: "README.md", MaxBytes: wireIntPointer(0)}},
		{shape: "UsageSummaryRequest", field: "sinceDays", value: UsageSummaryRequest{SinceDays: wireIntPointer(-1)}},
		{shape: "UsageSummaryRequest", field: "sinceDays", value: UsageSummaryRequest{SinceDays: wireIntPointer(0)}},
		{shape: "RuntimeEvent", field: "sequence", value: RuntimeEvent{Type: RuntimeSkillsChanged, Sequence: MaximumRuntimeEventSequence + 1}},
	} {
		t.Run(test.shape+"."+test.field, func(t *testing.T) {
			t.Parallel()
			assertConstraintField(t, test.value.ValidateWire(), test.shape, test.field)
		})
	}
}

func TestRunSummaryRequiresExecutionAttribution(t *testing.T) {
	t.Parallel()

	valid := RunSummary{
		ID: "run_1", SessionID: "ses_1", Provider: "provider", Model: "model",
		Status: RunStatusRunning, CreatedAt: time.Unix(1, 0).UTC(),
	}
	if err := valid.ValidateWire(); err != nil {
		t.Fatalf("valid RunSummary: %v", err)
	}
	for _, test := range []struct {
		field  string
		mutate func(*RunSummary)
	}{
		{field: "provider", mutate: func(run *RunSummary) { run.Provider = "" }},
		{field: "model", mutate: func(run *RunSummary) { run.Model = "" }},
		{field: "createdAt", mutate: func(run *RunSummary) { run.CreatedAt = time.Time{} }},
	} {
		t.Run(test.field, func(t *testing.T) {
			run := valid
			test.mutate(&run)
			assertConstraintField(t, run.ValidateWire(), "RunSummary", test.field)
		})
	}
}

func TestIntegerBoundsCompareWithoutFloat64Rounding(t *testing.T) {
	t.Parallel()

	// Adjacent uint64 values above JavaScript's exact-integer envelope collapse
	// to the same float64. The shared primitive must still distinguish them so a
	// future generated integer bound cannot become weaker than its schema.
	const lower uint64 = 1 << 53
	const upper uint64 = lower + 1
	if err := maximumNumber("value", upper, lower); err.Field != "value" {
		t.Fatalf("maximumNumber(%d, %d) = %+v, want violation", upper, lower, err)
	}
	if err := minimumNumber("value", lower, upper); err.Field != "value" {
		t.Fatalf("minimumNumber(%d, %d) = %+v, want violation", lower, upper, err)
	}
}

func TestRevisionWireConstraintsUseTheExactJSONEnvelope(t *testing.T) {
	t.Parallel()

	for name, value := range map[string]WireValidator{
		"Session": Session{
			ID: "ses_1", Status: SessionStatusIdle, Provider: "provider", Model: "model",
			Revision: MaximumExactJSONInteger,
		},
		"UpdateSessionRequest": UpdateSessionRequest{
			SessionID: "ses_1", ExpectedRevision: MaximumExactJSONInteger,
		},
		"Schedule": Schedule{
			ID: "sch_1", Instructions: "run", Cron: "@daily",
			Revision: MaximumExactJSONInteger,
		},
		"UpdateScheduleRequest": UpdateScheduleRequest{
			ID: "sch_1", ExpectedRevision: MaximumExactJSONInteger,
		},
		"PlanState": PlanState{Revision: MaximumExactJSONInteger},
	} {
		if err := value.ValidateWire(); err != nil {
			t.Fatalf("%s exact boundary: %v", name, err)
		}
	}

	for _, test := range []struct {
		name  string
		field string
		value WireValidator
	}{
		{name: "Session", field: "revision", value: Session{
			Status: SessionStatusIdle, Provider: "provider", Model: "model",
			Revision: MaximumExactJSONInteger + 1,
		}},
		{name: "UpdateSessionRequest", field: "expectedRevision", value: UpdateSessionRequest{
			SessionID: "ses_1", ExpectedRevision: MaximumExactJSONInteger + 1,
		}},
		{name: "Schedule", field: "revision", value: Schedule{
			ID: "sch_1", Instructions: "run", Cron: "@daily",
			Revision: MaximumExactJSONInteger + 1,
		}},
		{name: "UpdateScheduleRequest", field: "expectedRevision", value: UpdateScheduleRequest{
			ID: "sch_1", ExpectedRevision: MaximumExactJSONInteger + 1,
		}},
		{name: "PlanState", field: "revision", value: PlanState{Revision: MaximumExactJSONInteger + 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertConstraintField(t, test.value.ValidateWire(), test.name, test.field)
		})
	}
}

func TestPageContinuationBoundIsPromotedToEveryPageInstantiation(t *testing.T) {
	t.Parallel()

	boundary := strings.Repeat("x", MaximumPaginationCursorCharacters)
	if err := (Page[string]{PageContinuation: PageContinuation{NextCursor: boundary}}).ValidateWire(); err != nil {
		t.Fatalf("boundary page continuation: %v", err)
	}

	oversized := boundary + "x"
	for _, page := range []WireValidator{
		Page[string]{PageContinuation: PageContinuation{NextCursor: oversized}},
		Page[int]{PageContinuation: PageContinuation{NextCursor: oversized}},
	} {
		assertConstraintField(t, page.ValidateWire(), "PageContinuation", "nextCursor")
	}
}

func TestRunEventIdentityFramingDistinguishesAbsentFromMalformed(t *testing.T) {
	t.Parallel()

	if err := (RunEvent{RunID: "run_1", SegmentID: "seg_1", EventID: IDPrefixEvent + "opaque"}).ValidateWire(); err != nil {
		t.Fatalf("valid RunEvent identity: %v", err)
	}
	if err := (SubscribeRunResponse{RunID: "run_1", SegmentID: "seg_1"}).ValidateWire(); err != nil {
		t.Fatalf("absent head event identity: %v", err)
	}
	assertConstraintField(t,
		(SubscribeRunResponse{RunID: "run_1", SegmentID: "seg_1", HeadEventID: wireStringPointer("")}).ValidateWire(),
		"SubscribeRunResponse", "headEventId",
	)
}

func TestOperationalResourceIdentitiesAreExactAndBounded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		shape string
		field string
		value WireValidator
	}{
		{name: "session padding", shape: "GetSessionRequest", field: "sessionId", value: GetSessionRequest{SessionID: " ses_1"}},
		{name: "run whitespace", shape: "GetRunRequest", field: "runId", value: GetRunRequest{RunID: "run\n1"}},
		{name: "segment padding", shape: "SubscribeRunRequest", field: "segmentId", value: SubscribeRunRequest{RunID: "run_1", SegmentID: "seg_1 "}},
		{name: "session oversized", shape: "GetSessionRequest", field: "sessionId", value: GetSessionRequest{SessionID: strings.Repeat("界", MaximumResourceIdentityCharacters+1)}},
		{name: "run oversized", shape: "GetRunRequest", field: "runId", value: GetRunRequest{RunID: strings.Repeat("r", MaximumResourceIdentityCharacters+1)}},
		{name: "schedule prefix", shape: "DeleteScheduleRequest", field: "id", value: DeleteScheduleRequest{ID: "other_1"}},
		{name: "schedule whitespace", shape: "RunScheduleNowRequest", field: "id", value: RunScheduleNowRequest{ID: "sch_bad id"}},
		{name: "schedule oversized", shape: "UpdateScheduleRequest", field: "id", value: UpdateScheduleRequest{ID: IDPrefixSchedule + strings.Repeat("x", MaximumResourceIdentityCharacters), ExpectedRevision: 1}},
		{name: "schedule event prefix", shape: "RuntimeEvent", field: "scheduleIds[0]", value: RuntimeEvent{Type: RuntimeSchedulesChanged, Sequence: 1, ScheduleIDs: []string{"other_1"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertConstraintField(t, test.value.ValidateWire(), test.shape, test.field)
		})
	}
	assertConstraintField(t, ValidateWireTree(ResumeRunRequest{
		RunID: "run_1",
		Responses: []InterruptResponse{{
			ItemID: " item_1",
			Response: InterruptResponseValue{
				Type: InterruptResponseApproval, Decision: ApprovalApprove,
			},
		}},
	}), "ResumeRunRequest", "responses[0].itemId")
}

func TestWorkspaceReadWindowAndDiffFormatRejectMeaninglessFieldCombinations(t *testing.T) {
	t.Parallel()

	assertConstraintField(t, (ReadFileRequest{
		Path:    "README.md",
		EndLine: wireIntPointer(4),
	}).ValidateWire(), "ReadFileRequest", "startLine")
	assertConstraintField(t, (GetDiffRequest{
		Format: DiffFormatRaw,
		Limit:  wireIntPointer(4),
	}).ValidateWire(), "GetDiffRequest", "limit")
}

func wireIntPointer(value int) *int { return &value }

func wireStringPointer(value string) *string { return &value }

func TestGenerationAndGoalBoundsAreWireConstraints(t *testing.T) {
	t.Parallel()

	temperature := 2.1
	topP := 1.1
	zeroTokens := int64(0)
	zeroRuns := 0
	zeroCost := 0.0
	zeroSteps := 0
	for _, test := range []struct {
		field string
		value GenerationParams
	}{
		{field: "temperature", value: GenerationParams{Temperature: &temperature}},
		{field: "topP", value: GenerationParams{TopP: &topP}},
		{field: "maxTokens", value: GenerationParams{MaxTokens: &zeroTokens}},
		{field: "stop", value: GenerationParams{Stop: []string{}}},
	} {
		assertConstraintField(t, test.value.ValidateWire(), "GenerationParams", test.field)
	}

	assertConstraintField(t, (GoalBudget{}).ValidateWire(), "GoalBudget", "maxRuns|maxCostUsd|maxSteps")
	assertConstraintField(t, (GoalBudget{MaxRuns: &zeroRuns}).ValidateWire(), "GoalBudget", "maxRuns")
	assertConstraintField(t, (GoalBudget{MaxCostUSD: &zeroCost}).ValidateWire(), "GoalBudget", "maxCostUsd")
	assertConstraintField(t, (GoalBudget{MaxSteps: &zeroSteps}).ValidateWire(), "GoalBudget", "maxSteps")
	positiveFractionalCost := 0.25
	if err := (GoalBudget{MaxCostUSD: &positiveFractionalCost}).ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected a positive fractional cost: %v", err)
	}
	nonFiniteCost := math.Inf(1)
	assertConstraintField(t, (GoalBudget{MaxCostUSD: &nonFiniteCost}).ValidateWire(), "GoalBudget", "maxCostUsd")

	negativeCost := -0.01
	for _, test := range []struct {
		field string
		value GoalUsage
	}{
		{field: "runs", value: GoalUsage{Runs: -1}},
		{field: "costUsd", value: GoalUsage{CostUSD: &negativeCost}},
		{field: "steps", value: GoalUsage{Steps: -1}},
		{field: "costUsd", value: GoalUsage{CostUSD: &nonFiniteCost}},
	} {
		assertConstraintField(t, test.value.ValidateWire(), "GoalUsage", test.field)
	}

	goal := Goal{
		SessionID: "ses_1",
		Objective: "finish",
		Status:    GoalActive,
		Used:      GoalUsage{Steps: -1},
		CreatedAt: time.Unix(1, 0).UTC(),
		UpdatedAt: time.Unix(2, 0).UTC(),
	}
	assertConstraintField(t, ValidateWireTree(goal), "Goal", "used.steps")
}

func TestArtifactRunRequiresExecutionAttribution(t *testing.T) {
	t.Parallel()

	valid := ArtifactRun{ID: "run_1", SessionID: "ses_1", Provider: "provider", Model: "model"}
	if err := valid.ValidateWire(); err != nil {
		t.Fatalf("valid ArtifactRun: %v", err)
	}
	for _, test := range []struct {
		field  string
		mutate func(*ArtifactRun)
	}{
		{field: "provider", mutate: func(run *ArtifactRun) { run.Provider = "" }},
		{field: "model", mutate: func(run *ArtifactRun) { run.Model = "" }},
	} {
		t.Run(test.field, func(t *testing.T) {
			run := valid
			test.mutate(&run)
			assertConstraintField(t, run.ValidateWire(), "ArtifactRun", test.field)
		})
	}
}

func TestSessionArtifactBoundsAreWireConstraints(t *testing.T) {
	t.Parallel()

	artifact := SessionArtifact{Version: SessionArtifactVersion - 1}
	assertConstraintField(t, artifact.ValidateWire(), "SessionArtifact", "version")
	artifact.Version = SessionArtifactVersion + 1
	assertConstraintField(t, artifact.ValidateWire(), "SessionArtifact", "version")

	cost := -0.01
	tooLongDuration := MaximumDurationMilliseconds + 1
	for _, test := range []struct {
		shape string
		field string
		value WireValidator
	}{
		{shape: "ArtifactRun", field: "messageMark", value: ArtifactRun{
			ID: "run_1", SessionID: "ses_1", Provider: "provider", Model: "model", MessageMark: -1,
		}},
		{shape: "ArtifactRunMetrics", field: "steps", value: ArtifactRunMetrics{Steps: -1}},
		{shape: "ArtifactRunMetrics", field: "activeDurationMillis", value: ArtifactRunMetrics{ActiveDurationMillis: -1}},
		{shape: "ArtifactRunMetrics", field: "activeDurationMillis", value: ArtifactRunMetrics{ActiveDurationMillis: tooLongDuration}},
		{shape: "ArtifactUsage", field: "inputTokens", value: ArtifactUsage{InputTokens: -1}},
		{shape: "ArtifactUsage", field: "costUsd", value: ArtifactUsage{CostUSD: &cost}},
		{shape: "ArtifactModelUsage", field: "reasoningTokens", value: ArtifactModelUsage{ReasoningTokens: -1}},
		{shape: "ArtifactItem", field: "droppedMessages", value: ArtifactItem{DroppedMessages: -1}},
		{shape: "ArtifactItem", field: "durationMillis", value: ArtifactItem{DurationMillis: &tooLongDuration}},
		{shape: "ArtifactProblem", field: "retryAfterSeconds", value: ArtifactProblem{RetryAfterSeconds: -1}},
	} {
		assertConstraintField(t, test.value.ValidateWire(), test.shape, test.field)
	}
	if int64(math.MaxInt) > MaximumDurationSeconds {
		tooLongSeconds64 := MaximumDurationSeconds + 1
		tooLongSeconds := int(tooLongSeconds64)
		assertConstraintField(t, (ArtifactProblem{
			Type: ArtifactProblemTimeout, RetryAfterSeconds: tooLongSeconds,
		}).ValidateWire(), "ArtifactProblem", "retryAfterSeconds")
	}
	assertConstraintField(t, (ArtifactProblem{
		Type: ArtifactProblemToolFailed, RetryAfterSeconds: 1,
	}).ValidateWire(), "ArtifactProblem", "retryAfterSeconds")
	if err := (ArtifactProblem{
		Type: ArtifactProblemRateLimited, RetryAfterSeconds: 1,
	}).ValidateWire(); err != nil {
		t.Fatalf("transient Run retry hint validation error = %v", err)
	}
}

func TestSessionArtifactFailureTaxonomiesAreContextual(t *testing.T) {
	t.Parallel()

	at := time.Unix(1, 0).UTC()
	completedTool := ArtifactItem{
		ID: "item_1", RunID: "run_1", Status: ItemStatusCompleted,
		Type: ItemTypeToolCall, StartedAt: at, FinishedAt: at,
		Tool:  &ArtifactToolInvocation{Name: "shell", Arguments: map[string]any{}},
		Error: &ArtifactProblem{Type: ArtifactProblemToolFailed},
	}
	assertConstraintField(t, completedTool.ValidateWire(), "ArtifactItem", "error")

	incompleteTool := completedTool
	incompleteTool.Status = ItemStatusIncomplete
	incompleteTool.Error = &ArtifactProblem{Type: ArtifactProblemRunLost}
	assertConstraintField(t, incompleteTool.ValidateWire(), "ArtifactItem", "error.type")

	timedOut := ArtifactOutcome{
		Type:  ArtifactOutcomeTimedOut,
		Error: &ArtifactProblem{Type: ArtifactProblemProviderUnavailable},
	}
	assertConstraintField(t, timedOut.ValidateWire(), "ArtifactOutcome", "error.type")
}

func TestRuntimeOutputNumbersPreserveDomainBounds(t *testing.T) {
	t.Parallel()

	negative, tooManyTools := -1, mcpserver.MaxRemoteToolsPerServer+1
	negativeBytes := int64(-1)
	tooLongDuration := MaximumDurationMilliseconds + 1
	negativeCost, nonFiniteCost := -0.01, math.Inf(1)
	for _, test := range []struct {
		shape string
		field string
		value WireValidator
	}{
		{shape: "Item", field: "droppedMessages", value: Item{DroppedMessages: -1}},
		{shape: "Item", field: "durationMillis", value: Item{DurationMillis: &tooLongDuration}},
		{shape: "RunMetrics", field: "activeDurationMillis", value: RunMetrics{ActiveDurationMillis: tooLongDuration}},
		{shape: "WorkspaceSummary", field: "sessionCount", value: WorkspaceSummary{SessionCount: -1}},
		{shape: "FileContent", field: "totalLines", value: FileContent{}},
		{shape: "FileContent", field: "startLine", value: FileContent{TotalLines: 1, StartLine: -1}},
		{shape: "FileContent", field: "endLine", value: FileContent{TotalLines: 1, EndLine: -1}},
		{shape: "FileEntry", field: "sizeBytes", value: FileEntry{SizeBytes: &negativeBytes}},
		{shape: "FileLine", field: "lineNumber", value: FileLine{}},
		{shape: "GrepResult", field: "total", value: GrepResult{Total: -1}},
		{shape: "GrepMatch", field: "lineNumber", value: GrepMatch{}},
		{shape: "FileDiff", field: "added", value: FileDiff{Added: &negative}},
		{shape: "WorkspaceFileChange", field: "removed", value: WorkspaceFileChange{Removed: &negative}},
		{shape: "DiffRow", field: "leftLine", value: DiffRow{LeftLine: -1}},
		{shape: "UsageBucket", field: "runs", value: UsageBucket{Runs: -1}},
		{shape: "UsageSummary", field: "sessions", value: UsageSummary{Sessions: -1}},
		{shape: "UsageSummary", field: "runs", value: UsageSummary{Runs: -1}},
		{shape: "HookInfo", field: "timeoutMillis", value: HookInfo{TimeoutMillis: -1}},
		{shape: "HookInfo", field: "timeoutMillis", value: HookInfo{TimeoutMillis: hooks.MaxTimeoutMillis + 1}},
		{shape: "MCPServerState", field: "toolCount", value: MCPServerState{ToolCount: &negative}},
		{shape: "MCPServerState", field: "toolCount", value: MCPServerState{ToolCount: &tooManyTools}},
		{shape: "ModelPricing", field: "inputUsdPerMillionTokens", value: ModelPricing{InputUSDPerMillionTokens: negativeCost}},
		{shape: "ModelPricing", field: "outputUsdPerMillionTokens", value: ModelPricing{OutputUSDPerMillionTokens: nonFiniteCost}},
	} {
		t.Run(test.shape+"."+test.field, func(t *testing.T) {
			t.Parallel()
			assertConstraintField(t, test.value.ValidateWire(), test.shape, test.field)
		})
	}
}

func TestWorkspaceChangePathsAreRequired(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		shape string
		value WireValidator
	}{
		{shape: "FileDiff", value: FileDiff{Status: FileStatusModified}},
		{shape: "WorkspaceFileChange", value: WorkspaceFileChange{Status: FileStatusModified}},
	} {
		assertConstraintField(t, test.value.ValidateWire(), test.shape, "path")
	}
}

func TestReachableOutputLeavesKeepGeneratedConstraints(t *testing.T) {
	t.Parallel()

	assertConstraintField(t, ValidateWireTree(WorkspaceInfo{
		Availability: WorkspaceAvailability("unknown"),
	}), "WorkspaceInfo", "availability")
	assertConstraintField(t, (ListItemsResponse{
		Page: Page[Item]{PageContinuation: PageContinuation{
			NextCursor: strings.Repeat("a", MaximumPaginationCursorCharacters+1),
		}},
	}).ValidateWire(), "ListItemsResponse", "nextCursor")
}

func TestRuntimeOutputNumberBoundariesRemainRepresentable(t *testing.T) {
	t.Parallel()

	zero, maximumTools := 0, mcpserver.MaxRemoteToolsPerServer
	zeroBytes := int64(0)
	for _, value := range []WireValidator{
		WorkspaceSummary{},
		FileContent{TotalLines: 1, StartLine: 1, EndLine: 1},
		FileEntry{Type: FileEntryFile, SizeBytes: &zeroBytes},
		FileLine{LineNumber: 1},
		GrepResult{},
		GrepMatch{LineNumber: 1},
		FileDiff{Path: "main.go", Status: FileStatusModified, Added: &zero, Removed: &zero},
		WorkspaceFileChange{Path: "main.go", Status: FileStatusModified, Added: &zero, Removed: &zero},
		DiffRow{Type: DiffRowContext, LeftLine: 1, RightLine: 1, Code: "line"},
		UsageBucket{},
		UsageSummary{},
		HookInfo{Event: HookEventPreToolUse, Command: "true", TimeoutMillis: hooks.MaxTimeoutMillis, Scope: HookScopeGlobal, Source: "/hooks.json"},
		MCPServerState{Type: MCPServerConnected, ToolCount: &maximumTools},
		ModelPricing{},
	} {
		if err := value.ValidateWire(); err != nil {
			t.Errorf("ValidateWire rejected valid %T numeric boundaries: %v", value, err)
		}
	}
}

func TestHookInfoKeepsExecutableAndDeclarativeFormsDisjoint(t *testing.T) {
	t.Parallel()

	for _, hook := range []HookInfo{
		{Event: HookEventPreToolUse, Matcher: "shell*", Command: "check", Scope: HookScopeProject, Source: "/repo/.flame/hooks.json"},
		{Event: HookEventStop, Inject: "remember this", Scope: HookScopeGlobal, Source: "/home/.flame/hooks.json"},
	} {
		if err := hook.ValidateWire(); err != nil {
			t.Errorf("ValidateWire rejected valid hook %+v: %v", hook, err)
		}
	}

	for _, test := range []struct {
		field string
		hook  HookInfo
	}{
		{field: "command|inject", hook: HookInfo{Event: HookEventStop, Scope: HookScopeGlobal, Source: "/hooks.json"}},
		{field: "inject", hook: HookInfo{Event: HookEventStop, Command: "check", Inject: "context", Scope: HookScopeGlobal, Source: "/hooks.json"}},
		{field: "timeoutMillis", hook: HookInfo{Event: HookEventStop, Inject: "context", TimeoutMillis: 1, Scope: HookScopeGlobal, Source: "/hooks.json"}},
		{field: "matcher", hook: HookInfo{Event: HookEventStop, Matcher: "shell*", Command: "check", Scope: HookScopeGlobal, Source: "/hooks.json"}},
		{field: "source", hook: HookInfo{Event: HookEventStop, Command: "check", Scope: HookScopeGlobal}},
	} {
		assertConstraintField(t, test.hook.ValidateWire(), "HookInfo", test.field)
	}
}

func TestFileContentWindowBoundariesAreAtomic(t *testing.T) {
	t.Parallel()

	if err := (FileContent{TotalLines: 1}).ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected an unwindowed file: %v", err)
	}
	if err := (FileContent{TotalLines: 1, StartLine: 1, EndLine: 1}).ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected a complete window: %v", err)
	}
	assertConstraintField(t, (FileContent{TotalLines: 1, StartLine: 1}).ValidateWire(), "FileContent", "endLine")
	assertConstraintField(t, (FileContent{TotalLines: 1, EndLine: 1}).ValidateWire(), "FileContent", "startLine")
}

func TestWorkspaceChangeMetadataMatchesItsStatusAndRepresentation(t *testing.T) {
	t.Parallel()

	count := 1
	for _, value := range []WireValidator{
		WorkspaceFileChange{Path: "main.go", Status: FileStatusRenamed, PreviousPath: "old.go"},
		WorkspaceFileChange{Path: "main.go", Status: FileStatusModified, Binary: true},
		FileDiff{Path: "main.go", Status: FileStatusRenamed, PreviousPath: "old.go", Rows: []DiffRow{}},
		FileDiff{Path: "main.go", Status: FileStatusModified, Binary: true, Rows: []DiffRow{}},
	} {
		if err := value.ValidateWire(); err != nil {
			t.Errorf("ValidateWire rejected valid %T metadata: %v", value, err)
		}
	}

	for _, test := range []struct {
		shape string
		field string
		value WireValidator
	}{
		{shape: "WorkspaceFileChange", field: "previousPath", value: WorkspaceFileChange{Path: "main.go", Status: FileStatusRenamed}},
		{shape: "WorkspaceFileChange", field: "previousPath", value: WorkspaceFileChange{Path: "main.go", Status: FileStatusModified, PreviousPath: "old.go"}},
		{shape: "WorkspaceFileChange", field: "added", value: WorkspaceFileChange{Path: "main.go", Status: FileStatusModified, Binary: true, Added: &count}},
		{shape: "FileDiff", field: "previousPath", value: FileDiff{Path: "main.go", Status: FileStatusRenamed}},
		{shape: "FileDiff", field: "previousPath", value: FileDiff{Path: "main.go", Status: FileStatusUntracked, PreviousPath: "old.go"}},
		{shape: "FileDiff", field: "removed", value: FileDiff{Path: "main.go", Status: FileStatusModified, Binary: true, Removed: &count}},
	} {
		assertConstraintField(t, test.value.ValidateWire(), test.shape, test.field)
	}
}

func TestOptionalMCPUpdateConstraintsPreserveAndValidatePresentValues(t *testing.T) {
	t.Parallel()

	request := UpdateMCPServerRequest{Server: "files"}
	if err := request.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected an omission-only patch: %v", err)
	}

	negative := -1
	timeout := MCPHandshakeTimeout{Type: MCPHandshakeBounded, Seconds: &negative}
	assertConstraintField(t, timeout.ValidateWire(), "MCPHandshakeTimeout", "seconds")

	repeated := []string{"read", "read"}
	request.DisabledTools = &repeated
	assertConstraintField(t, request.ValidateWire(), "UpdateMCPServerRequest", "disabledTools")
}

func assertConstraintField(t *testing.T, err error, shape, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("ValidateWire accepted invalid %s.%s", shape, field)
	}
	constraint, ok := errors.AsType[*ConstraintError](err)
	if !ok {
		t.Fatalf("ValidateWire error = %T %v, want *ConstraintError", err, err)
	}
	for _, violation := range constraint.Fields {
		if violation.Field == field {
			if !strings.Contains(err.Error(), shape+"."+field) {
				t.Fatalf("error = %q, want shape-qualified path %s.%s", err, shape, field)
			}
			return
		}
	}
	t.Fatalf("violations = %+v, want field %q", constraint.Fields, field)
}
