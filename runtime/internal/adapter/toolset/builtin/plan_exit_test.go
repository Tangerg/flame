package builtin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	plandomain "github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

type modeStore struct {
	states map[string]approval.SessionMode
}

func (m *modeStore) LookupMode(_ context.Context, sessionID string) (approval.SessionMode, bool, error) {
	state, found := m.states[sessionID]
	return state, found, nil
}

func (m *modeStore) PutMode(_ context.Context, sessionID string, state approval.SessionMode) error {
	m.states[sessionID] = state
	return nil
}

func (m *modeStore) DeleteSession(_ context.Context, sessionID string) error {
	delete(m.states, sessionID)
	return nil
}

type planReader struct{ steps []plandomain.Step }

func (p planReader) State(context.Context, string) (plandomain.Current, error) {
	if len(p.steps) == 0 {
		return plandomain.Current{}, nil
	}
	state, err := plandomain.Restore(plandomain.Snapshot{Steps: p.steps, Revision: 1, UpdatedAt: time.Now()})
	if err != nil {
		return plandomain.Current{}, err
	}
	return plandomain.CurrentOf(state)
}

func planContext(t *testing.T, sessionID string) context.Context {
	t.Helper()
	return executionctx.WithScope(t.Context(), runs.ExecutionScope{SessionID: sessionID})
}

func balancedPlanPolicy(t *testing.T) *approvals.RuntimePolicy {
	t.Helper()
	policy, err := approvals.NewRuntimePolicy(approval.ModeBalanced, nil, &modeStore{states: make(map[string]approval.SessionMode)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func TestNewRequiresModeAndPlanPorts(t *testing.T) {
	policy := balancedPlanPolicy(t)
	for _, build := range []func() (any, error){
		func() (any, error) { return newExit(nil, planReader{}, nil) },
		func() (any, error) { return newExit(policy, nil, nil) },
	} {
		got, err := build()
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Fatal("missing port should omit exit_plan_mode")
		}
	}
}

func TestDefinitionHasNoSecondPlanInput(t *testing.T) {
	tool, err := newExit(balancedPlanPolicy(t), planReader{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := tool.Definition().Name; got != "exit_plan_mode" {
		t.Fatalf("name = %q", got)
	}
	schema := string(tool.Definition().InputSchema)
	if strings.Contains(schema, `"plan"`) || strings.Contains(schema, `"options"`) {
		t.Fatalf("schema exposes a second Plan value: %s", schema)
	}
	if _, err := callTextTool(t.Context(), tool, `{"plan":"shadow copy"}`); err == nil {
		t.Fatal("obsolete Plan input was accepted")
	}
}

func TestExitRequiresSessionAndPlanMode(t *testing.T) {
	policy := balancedPlanPolicy(t)
	tool, err := newExit(policy, planReader{steps: []plandomain.Step{{Description: "inspect", Status: plandomain.StatusPending}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := callTextTool(t.Context(), tool, `{}`); err == nil || !strings.Contains(err.Error(), "no active session") {
		t.Fatalf("missing-session error = %v", err)
	}
	if _, err := callTextTool(planContext(t, "session-1"), tool, `{}`); err == nil || !strings.Contains(err.Error(), "not in Plan mode") {
		t.Fatalf("non-Plan error = %v", err)
	}
}

func TestExitRejectsEmptyCanonicalPlan(t *testing.T) {
	policy := balancedPlanPolicy(t)
	if _, err := policy.EnterPlanMode(t.Context(), "session-1"); err != nil {
		t.Fatal(err)
	}
	tool, err := newExit(policy, planReader{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := callTextTool(planContext(t, "session-1"), tool, `{}`); err == nil || !strings.Contains(err.Error(), "current Plan is empty") {
		t.Fatalf("empty Plan error = %v", err)
	}
}

func TestRejectionKeepsPlanMode(t *testing.T) {
	const sessionID = "session-reject"
	policy := balancedPlanPolicy(t)
	if _, err := policy.EnterPlanMode(t.Context(), sessionID); err != nil {
		t.Fatal(err)
	}
	interrupt := func(_ context.Context, _ string, pending runs.Interrupt) (interrupt.Resolution, error) {
		if pending.Question == nil || !strings.Contains(pending.Question.Fields[0].Prompt, "inspect") {
			t.Fatalf("prompt does not present the canonical Plan: %+v", pending.Question)
		}
		return interrupt.Resolution{Answers: [][]string{{rejectLabel}}}, nil
	}
	tool, err := newExit(policy, planReader{steps: []plandomain.Step{{Description: "inspect", Status: plandomain.StatusInProgress}}}, interrupt)
	if err != nil {
		t.Fatal(err)
	}
	result, err := callTextTool(planContext(t, sessionID), tool, `{}`)
	if err != nil || !strings.Contains(result, "remains in Plan mode") {
		t.Fatalf("Call = %q, %v", result, err)
	}
	if mode, _ := policy.Mode(t.Context(), sessionID); mode != approval.ModePlan {
		t.Fatalf("mode = %v, want Plan", mode)
	}
}

func TestApprovalRestoresModeCapturedOnEntry(t *testing.T) {
	const sessionID = "session-approve"
	policy := balancedPlanPolicy(t)
	if _, err := policy.EnterPlanMode(t.Context(), sessionID); err != nil {
		t.Fatal(err)
	}
	if err := policy.SetDefaultMode(t.Context(), approval.ModeYolo); err != nil {
		t.Fatal(err)
	}
	interrupt := func(_ context.Context, _ string, pending runs.Interrupt) (interrupt.Resolution, error) {
		if got := pending.Question.Arguments; got != `{}` {
			t.Fatalf("interrupt arguments = %q", got)
		}
		return interrupt.Resolution{Answers: [][]string{{approveLabel}}}, nil
	}
	tool, err := newExit(policy, planReader{steps: []plandomain.Step{{Description: "implement", Status: plandomain.StatusPending}}}, interrupt)
	if err != nil {
		t.Fatal(err)
	}
	result, err := callTextTool(planContext(t, sessionID), tool, `{}`)
	if err != nil || !strings.Contains(result, "balanced was restored") {
		t.Fatalf("Call = %q, %v", result, err)
	}
	if mode, _ := policy.Mode(t.Context(), sessionID); mode != approval.ModeBalanced {
		t.Fatalf("mode = %v, want captured Balanced", mode)
	}
}
