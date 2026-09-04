package approvals

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
)

func TestDefaultModeGetSet(t *testing.T) {
	policy := mustStorelessRuntimePolicy(t, approval.ModeYolo)
	if mode, _ := policy.DefaultMode(t.Context()); mode != approval.ModeYolo {
		t.Fatalf("initial mode = %v, want Yolo", mode)
	}
	if err := policy.SetDefaultMode(t.Context(), approval.ModeBalanced); err != nil {
		t.Fatalf("SetDefaultMode: %v", err)
	}
	if mode, _ := policy.Mode(t.Context(), "session-1"); mode != approval.ModeBalanced {
		t.Fatalf("mode after set = %v, want Balanced", mode)
	}
}

func TestNilRuleStore(t *testing.T) {
	policy := mustStorelessRuntimePolicy(t, approval.ModeSafe)
	request := approval.RememberRequest{
		Scope: approval.ScopeGlobal, Tool: "shell", Subject: "go test", Decision: approval.Allow,
	}
	if err := policy.Remember(t.Context(), request); !errors.Is(err, ErrRuleStoreUnavailable) {
		t.Fatalf("Remember error = %v, want ErrRuleStoreUnavailable", err)
	}
	if _, ok, _ := policy.Decide(t.Context(), approval.Query{Tool: "shell", Subject: "go test"}); ok {
		t.Fatal("nil store matched a rule")
	}
	if rules, _ := policy.Rules(t.Context(), "s1", "/p"); rules != nil {
		t.Fatalf("Rules = %+v, want nil", rules)
	}
	if err := policy.Forget(t.Context(), "rule_missing"); !errors.Is(err, ErrRuleStoreUnavailable) {
		t.Fatalf("Forget error = %v, want ErrRuleStoreUnavailable", err)
	}
}

func TestPolicyRejectsInvalidDefaultMode(t *testing.T) {
	if _, err := NewRuntimePolicy(approval.Mode("invalid"), nil, nil, nil); !errors.Is(err, approval.ErrInvalidMode) {
		t.Fatalf("New invalid mode error = %v, want ErrInvalidMode", err)
	}
	if _, err := NewRuntimePolicy(approval.ModePlan, nil, nil, nil); !errors.Is(err, approval.ErrInvalidMode) {
		t.Fatalf("New Plan default error = %v, want ErrInvalidMode", err)
	}
	policy := mustStorelessRuntimePolicy(t, approval.ModeSafe)
	if err := policy.SetDefaultMode(t.Context(), approval.Mode("invalid")); !errors.Is(err, approval.ErrInvalidMode) {
		t.Fatalf("SetDefaultMode error = %v, want ErrInvalidMode", err)
	}
	if got, err := policy.Mode(t.Context(), ""); err != nil || got != approval.ModeSafe {
		t.Fatalf("mode after rejected update = (%v, %v), want Safe", got, err)
	}
}

type memoryModeStore struct {
	states map[string]approval.SessionMode
}

func (m *memoryModeStore) LookupMode(_ context.Context, sessionID string) (approval.SessionMode, bool, error) {
	state, found := m.states[sessionID]
	return state, found, nil
}

func (m *memoryModeStore) PutMode(_ context.Context, sessionID string, state approval.SessionMode) error {
	m.states[sessionID] = state
	return nil
}

func (m *memoryModeStore) DeleteSession(_ context.Context, sessionID string) error {
	delete(m.states, sessionID)
	return nil
}

func TestPlanModeIsSessionScopedAndRestoresEntryMode(t *testing.T) {
	modes := &memoryModeStore{states: make(map[string]approval.SessionMode)}
	policy, err := NewRuntimePolicy(approval.ModeBalanced, nil, modes, nil)
	if err != nil {
		t.Fatal(err)
	}
	if changed, enterPlanModeErr := policy.EnterPlanMode(t.Context(), "session-a"); enterPlanModeErr != nil || !changed {
		t.Fatalf("EnterPlanMode = %v, %v", changed, enterPlanModeErr)
	}
	if mode, _ := policy.Mode(t.Context(), "session-a"); mode != approval.ModePlan {
		t.Fatalf("session-a mode = %v, want Plan", mode)
	}
	if mode, _ := policy.Mode(t.Context(), "session-b"); mode != approval.ModeBalanced {
		t.Fatalf("session-b mode = %v, want Balanced", mode)
	}
	if setDefaultModeErr := policy.SetDefaultMode(t.Context(), approval.ModeYolo); setDefaultModeErr != nil {
		t.Fatal(setDefaultModeErr)
	}
	if changed, enterPlanModeErr := policy.EnterPlanMode(t.Context(), "session-a"); enterPlanModeErr != nil || changed {
		t.Fatalf("second EnterPlanMode = %v, %v, want unchanged", changed, enterPlanModeErr)
	}
	restored, changed, err := policy.ExitPlanMode(t.Context(), "session-a")
	if err != nil || !changed || restored != approval.ModeBalanced {
		t.Fatalf("ExitPlanMode = %v, %v, %v", restored, changed, err)
	}
	if mode, _ := policy.Mode(t.Context(), "session-a"); mode != approval.ModeBalanced {
		t.Fatalf("restored session-a mode = %v, want Balanced", mode)
	}
	if mode, _ := policy.Mode(t.Context(), "session-b"); mode != approval.ModeYolo {
		t.Fatalf("session-b mode = %v, want Yolo", mode)
	}
}

func TestPlanModeRequiresDurableStore(t *testing.T) {
	policy := mustStorelessRuntimePolicy(t, approval.ModeBalanced)
	if _, err := policy.EnterPlanMode(t.Context(), "session-a"); !errors.Is(err, ErrModeStoreUnavailable) {
		t.Fatalf("EnterPlanMode error = %v, want ErrModeStoreUnavailable", err)
	}
	if _, _, err := policy.ExitPlanMode(t.Context(), "session-a"); !errors.Is(err, ErrModeStoreUnavailable) {
		t.Fatalf("ExitPlanMode error = %v, want ErrModeStoreUnavailable", err)
	}
}

type ruleStoreStub struct {
	rules        []approval.Rule
	err          error
	visibleCalls *int
	visibleLimit *int
}

func (r ruleStoreStub) Put(context.Context, approval.Rule) error { return r.err }
func (r ruleStoreStub) Visible(_ context.Context, _, _ string, limit int) ([]approval.Rule, error) {
	if r.visibleCalls != nil {
		*r.visibleCalls++
	}
	if r.visibleLimit != nil {
		*r.visibleLimit = limit
	}
	return r.rules, r.err
}
func (r ruleStoreStub) Delete(context.Context, string) error { return r.err }

func TestRuntimePolicyRejectsInvalidQueryBeforeRuleStore(t *testing.T) {
	calls := 0
	policy, err := NewRuntimePolicy(approval.ModeSafe, ruleStoreStub{visibleCalls: &calls}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := policy.Decide(t.Context(), approval.Query{Tool: " shell"}); !errors.Is(err, approval.ErrInvalidQuery) {
		t.Fatalf("Decide error = %v, want ErrInvalidQuery", err)
	}
	if calls != 0 {
		t.Fatalf("Visible calls = %d, want 0", calls)
	}
}

func TestRuntimePolicyOverfetchesAndRejectsRuleCapacity(t *testing.T) {
	limit := 0
	policy, err := NewRuntimePolicy(approval.ModeSafe, ruleStoreStub{
		rules: make([]approval.Rule, approval.MaximumVisibleRules+1), visibleLimit: &limit,
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Rules(t.Context(), "s1", "/repo"); !errors.Is(err, approval.ErrRuleCapacity) {
		t.Fatalf("Rules error = %v, want ErrRuleCapacity", err)
	}
	if limit != approval.MaximumVisibleRules+1 {
		t.Fatalf("Visible limit = %d, want one-row overfetch", limit)
	}
}

func TestRuntimePolicyProtectsVisibleRuleRelations(t *testing.T) {
	visible := mustRuntimePolicyRule(t, approval.ScopeSession, "s1", approval.Allow)
	tests := []struct {
		name  string
		rules []approval.Rule
	}{
		{
			name:  "other session",
			rules: []approval.Rule{mustRuntimePolicyRule(t, approval.ScopeSession, "s2", approval.Allow)},
		},
		{
			name:  "other project",
			rules: []approval.Rule{mustRuntimePolicyRule(t, approval.ScopeProject, "/other", approval.Allow)},
		},
		{name: "duplicate identity", rules: []approval.Rule{visible, visible}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy, err := NewRuntimePolicy(approval.ModeSafe, ruleStoreStub{rules: test.rules}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			query := approval.Query{SessionID: "s1", ProjectDir: "/repo", Tool: "shell"}
			if _, _, err := policy.Decide(t.Context(), query); !errors.Is(err, approval.ErrInvalidRule) {
				t.Fatalf("Decide error = %v, want ErrInvalidRule", err)
			}
			if _, err := policy.Rules(t.Context(), query.SessionID, query.ProjectDir); !errors.Is(err, approval.ErrInvalidRule) {
				t.Fatalf("Rules error = %v, want ErrInvalidRule", err)
			}
		})
	}
}

func TestRuntimePolicyIsolatesVisibleRuleStorage(t *testing.T) {
	rule := mustRuntimePolicyRule(t, approval.ScopeGlobal, "", approval.Allow)
	policy, err := NewRuntimePolicy(approval.ModeSafe, ruleStoreStub{rules: []approval.Rule{rule}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := policy.Rules(t.Context(), "s1", "/repo")
	if err != nil {
		t.Fatal(err)
	}
	rules[0] = approval.Rule{}

	got, err := policy.Rules(t.Context(), "s1", "/repo")
	if err != nil {
		t.Fatalf("Rules after caller mutation: %v", err)
	}
	if len(got) != 1 || got[0] != rule {
		t.Fatalf("Rules after caller mutation = %+v, want %+v", got, rule)
	}
}

func mustRuntimePolicyRule(t *testing.T, scope approval.Scope, scopeKey string, decision approval.Decision) approval.Rule {
	t.Helper()
	rule, err := approval.NewRule(scope, scopeKey, "shell", "", decision)
	if err != nil {
		t.Fatalf("NewRule: %v", err)
	}
	return rule
}

func TestCommittedApprovalMutationsPublishInvalidations(t *testing.T) {
	var notices []invalidation.Notice
	policy, err := NewRuntimePolicy(
		approval.ModeSafe,
		ruleStoreStub{},
		nil,
		func(notice invalidation.Notice) { notices = append(notices, notice) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.SetDefaultMode(t.Context(), approval.ModeBalanced); err != nil {
		t.Fatal(err)
	}
	if err := policy.Remember(t.Context(), approval.RememberRequest{
		Scope: approval.ScopeGlobal, Tool: "shell", Subject: "go test", Decision: approval.Allow,
	}); err != nil {
		t.Fatal(err)
	}
	if err := policy.Forget(t.Context(), "rule_1"); err != nil {
		t.Fatal(err)
	}
	if len(notices) != 3 {
		t.Fatalf("notices = %+v, want three", notices)
	}
	for _, notice := range notices {
		if notice.Resource != invalidation.Approvals {
			t.Fatalf("notice = %+v, want approvals", notice)
		}
	}
}

func TestFailedApprovalMutationDoesNotPublishInvalidation(t *testing.T) {
	wantErr := errors.New("store unavailable")
	var notices []invalidation.Notice
	policy, err := NewRuntimePolicy(
		approval.ModeSafe,
		ruleStoreStub{err: wantErr},
		nil,
		func(notice invalidation.Notice) { notices = append(notices, notice) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.Remember(t.Context(), approval.RememberRequest{
		Scope: approval.ScopeGlobal, Tool: "shell", Subject: "go test", Decision: approval.Allow,
	}); !errors.Is(err, wantErr) {
		t.Fatalf("Remember error = %v, want %v", err, wantErr)
	}
	if err := policy.Forget(t.Context(), "rule_1"); !errors.Is(err, wantErr) {
		t.Fatalf("Forget error = %v, want %v", err, wantErr)
	}
	if len(notices) != 0 {
		t.Fatalf("failed mutations published %+v", notices)
	}
}

func mustStorelessRuntimePolicy(t *testing.T, mode approval.Mode) *RuntimePolicy {
	t.Helper()
	policy, err := NewRuntimePolicy(mode, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewRuntimePolicy: %v", err)
	}
	return policy
}
