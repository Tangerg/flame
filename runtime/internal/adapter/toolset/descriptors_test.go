package toolset

import (
	"context"
	"crypto/sha256"

	"fmt"
	"path/filepath"

	"testing"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/builtin"

	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/goals"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type activeGoalStub struct{}

func (activeGoalStub) Current(context.Context, string) (goal.Goal, bool, error) {
	return goal.Goal{}, false, nil
}
func (activeGoalStub) Report(context.Context, goals.ReportCommand) (goals.ReportResult, error) {
	return goals.ReportNoActiveGoal, nil
}

type allWiredGoalStarter struct{}

func (allWiredGoalStarter) Start(context.Context, string, string, modelchoice.Selection, run.Capabilities) (goal.Goal, error) {
	return goal.Goal{}, nil
}

func wireCreateGoal(t *testing.T, resolver *Resolver) {
	t.Helper()
	create, err := builtin.NewCreate(allWiredGoalStarter{})
	if err != nil {
		t.Fatalf("build create_goal: %v", err)
	}
	resolver.UseCreateGoalTool(create)
}

type allWiredSkillProposals struct{}

func (allWiredSkillProposals) SubmitProposal(_ context.Context, _ string, proposal skills.Proposal) (skills.ProposalRef, error) {
	return skills.CommandRef(proposal.Scope, proposal.Name, fmt.Sprintf("%x", sha256.Sum256([]byte(proposal.Instructions)))), nil
}

func definitionNames(ts []toolcontract.Tool) map[string]bool {
	names := make(map[string]bool, len(ts))
	for _, t := range ts {
		names[t.Definition().Name] = true
	}
	return names
}

func TestRootResolverIncludesConfiguredConditionalTools(t *testing.T) {
	policy := testApprovalPolicy(t)
	built, err := Build(BuildConfig{Lifetime: t.Context(),
		UserHome:     t.TempDir(),
		PlanMode:     policy,
		Plan:         rolePlanStore{},
		GoalReader:   activeGoalStub{},
		GoalReporter: activeGoalStub{},
		Interrupt: func(context.Context, toolcontract.Invocation, runs.Interrupt) (run.InterruptResolution, *toolcontract.InputRequest, error) {
			return run.InterruptResolution{}, nil, nil
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)
	wireCreateGoal(t, built.Resolver)

	goalRunContext := executionctx.WithScope(t.Context(), runs.ExecutionScope{
		SessionID: "session-goal", GoalIncarnationID: "incarnation-1",
		CWD: t.TempDir(), WorkspaceCWD: t.TempDir(),
	})
	manifest, err := built.Resolver.Manifest(goalRunContext, tool.GroupRoot, nil)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	t.Cleanup(func() {
		if err := manifest.Close(); err != nil {
			t.Error(err)
		}
	})
	names := definitionNames(manifestTools(manifest))
	for _, want := range []string{"enter_plan_mode", "exit_plan_mode", "create_goal", "get_goal", "report_goal_outcome"} {
		if !names[want] {
			t.Errorf("configured root tools missing %q: %v", want, names)
		}
	}
}

func testApprovalPolicy(t *testing.T) *approvals.RuntimePolicy {
	t.Helper()
	db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(filepath.Join(t.TempDir(), "approvals.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	policy, err := approvals.NewRuntimePolicy(approval.ModeBalanced, db.ApprovalRules, db.ApprovalModes, db.Sessions, testsupport.ToolAuthorities{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}
