package approval_test

import (
	"errors"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func TestStandingRuleDoesNotFollowTheModelName(t *testing.T) {
	remote, err := tool.A2A("shell")
	if err != nil {
		t.Fatal(err)
	}
	builtin := testsupport.BuiltInTool(t, "shell")
	if remote.ModelName() != builtin.ModelName() {
		t.Fatal("fixture does not collide")
	}
	rule := mustRule(t, approval.ScopeGlobal, "", "shell", approval.InvocationSubject(""), approval.Allow)
	if _, matched, err := approval.Decide([]approval.Rule{rule}, approval.Query{Tool: remote, SourceFingerprint: testsupport.ToolFingerprint(remote)}); err != nil || matched {
		t.Fatalf("remote identity inherited built-in approval: matched=%t, err=%v", matched, err)
	}
}

// White-box tests: the matching/precedence rules are the heart of the rule
// engine, so they're exercised directly alongside the policy round-trips.

func TestRuleMatchesSubject(t *testing.T) {
	cases := []struct {
		matcher approval.Subject
		subject string
		want    bool
	}{
		{approval.Subject{Type: approval.SubjectAll}, "anything", true},
		{approval.InvocationSubject("npm run build"), "npm run build", true},
		{approval.InvocationSubject("npm run build"), "npm test", false},
		{approval.InvocationSubject("echo *"), "echo original", false},
		{approval.InvocationSubject("echo ["), "echo [", true},
		{approval.InvocationSubject("[draft]?.go"), "[draft]?.go", true},
		{approval.Subject{Type: approval.SubjectGlob, Value: "npm run *"}, "npm run build", true},
		{approval.Subject{Type: approval.SubjectGlob, Value: "npm run *"}, "yarn build", false},
		{approval.Subject{Type: approval.SubjectGlob, Value: "src/*.go"}, "src/a.go", true},
		{approval.Subject{Type: approval.SubjectGlob, Value: "src/*.go"}, "src/sub/a.go", false},
	}
	for _, c := range cases {
		rule := mustRule(t, approval.ScopeGlobal, "", "shell", c.matcher, approval.Allow)
		_, got, err := approval.Decide([]approval.Rule{rule}, approval.Query{Tool: testsupport.BuiltInTool(t, "shell"), Subject: c.subject})
		if err != nil || got != c.want {
			t.Errorf("Decide subject(%+v,%q) = %v, %v; want %v", c.matcher, c.subject, got, err, c.want)
		}
	}
}

func TestSubjectRejectsImplicitOrAmbiguousMatchers(t *testing.T) {
	for _, subject := range []approval.Subject{
		{},
		{Type: "unknown", Value: "go test"},
		{Type: approval.SubjectAll, Value: "go test"},
		{Type: approval.SubjectExact},
		{Type: approval.SubjectGlob},
		{Type: approval.SubjectGlob, Value: "echo ["},
	} {
		if err := subject.Validate(); !errors.Is(err, approval.ErrInvalidRule) {
			t.Errorf("Validate(%+v) = %v, want ErrInvalidRule", subject, err)
		}
	}
}

// TestDecidePrecedence: the most specific matching rule wins — scope dominates
// (session > project > global), then subject (exact > glob > any).
func TestDecidePrecedence(t *testing.T) {
	q := approval.Query{SessionID: "s1", ProjectDir: "/p", Tool: testsupport.BuiltInTool(t, "shell"), Subject: "rm -rf /", SourceFingerprint: testsupport.ToolFingerprint(testsupport.BuiltInTool(t, "shell"))}

	// A broad session allow vs a narrow (exact-subject) session deny → deny wins.

	rules := []approval.Rule{
		mustRule(t, approval.ScopeSession, "s1", "shell", approval.InvocationSubject(""), approval.Allow),
		mustRule(t, approval.ScopeSession, "s1", "shell", approval.InvocationSubject("rm -rf /"), approval.Deny),
	}
	if d, ok, err := approval.Decide(rules, q); err != nil || !ok || d != approval.Deny {
		t.Fatalf("exact deny over broad allow = (%v,%v,%v), want (deny,true,nil)", d, ok, err)
	}

	// A global deny vs a session allow (both whole-tool) → session allow wins.
	rules = []approval.Rule{
		mustRule(t, approval.ScopeGlobal, "", "shell", approval.InvocationSubject(""), approval.Deny),
		mustRule(t, approval.ScopeSession, "s1", "shell", approval.InvocationSubject(""), approval.Allow),
	}
	if d, ok, err := approval.Decide(rules, q); err != nil || !ok || d != approval.Allow {
		t.Fatalf("session allow over global deny = (%v,%v,%v), want (allow,true,nil)", d, ok, err)
	}

	// Wrong tool / no rules → miss.
	if _, ok, err := approval.Decide([]approval.Rule{mustRule(t, approval.ScopeSession, "s1", "edit", approval.InvocationSubject(""), approval.Allow)}, q); err != nil || ok {
		t.Fatal("a write rule matched a shell call")
	}
}

// Distinct equally specific patterns that match the same subject resolve to deny.
func TestDecideConflictDeny(t *testing.T) {
	q := approval.Query{SessionID: "s1", Tool: testsupport.BuiltInTool(t, "shell"), Subject: "go test", SourceFingerprint: testsupport.ToolFingerprint(testsupport.BuiltInTool(t, "shell"))}
	rules := []approval.Rule{
		mustRule(t, approval.ScopeSession, "s1", "shell", approval.Subject{Type: approval.SubjectGlob, Value: "go *"}, approval.Allow),
		mustRule(t, approval.ScopeSession, "s1", "shell", approval.Subject{Type: approval.SubjectGlob, Value: "* test"}, approval.Deny),
	}
	if d, ok, err := approval.Decide(rules, q); err != nil || !ok || d != approval.Deny {
		t.Fatalf("conflict = (%v,%v,%v), want (deny,true,nil)", d, ok, err)
	}
}

func TestDecideRejectsInvalidVisibleRuleRelation(t *testing.T) {
	q := approval.Query{SessionID: "s1", ProjectDir: "/repo", Tool: testsupport.BuiltInTool(t, "shell"), Subject: "go test", SourceFingerprint: testsupport.ToolFingerprint(testsupport.BuiltInTool(t, "shell"))}
	visible := mustRule(t, approval.ScopeSession, "s1", "shell", approval.InvocationSubject(""), approval.Allow)
	tests := []struct {
		name  string
		rules []approval.Rule
	}{
		{
			name:  "other session",
			rules: []approval.Rule{mustRule(t, approval.ScopeSession, "s2", "shell", approval.InvocationSubject(""), approval.Allow)},
		},
		{
			name:  "other project",
			rules: []approval.Rule{mustRule(t, approval.ScopeProject, "/other", "shell", approval.InvocationSubject(""), approval.Allow)},
		},
		{name: "duplicate identity", rules: []approval.Rule{visible, visible}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := approval.Decide(test.rules, q); !errors.Is(err, approval.ErrInvalidRule) {
				t.Fatalf("Decide error = %v, want ErrInvalidRule", err)
			}
		})
	}
}

func TestVisibleRuleRelationRejectsOverCapacity(t *testing.T) {
	rules := make([]approval.Rule, approval.MaximumVisibleRules+1)
	if err := approval.ValidateVisibleRules(rules, "s1", "/repo"); !errors.Is(err, approval.ErrRuleCapacity) {
		t.Fatalf("ValidateVisibleRules error = %v, want ErrRuleCapacity", err)
	}
	if _, _, err := approval.Decide(rules, approval.Query{SessionID: "s1", ProjectDir: "/repo", Tool: testsupport.BuiltInTool(t, "shell"), SourceFingerprint: testsupport.ToolFingerprint(testsupport.BuiltInTool(t, "shell"))}); !errors.Is(err, approval.ErrRuleCapacity) {
		t.Fatalf("Decide error = %v, want ErrRuleCapacity", err)
	}
}

func TestSessionScopedApprovalIdentityIsExact(t *testing.T) {
	if _, err := approval.NewRule(approval.ScopeSession, "ses_ one", testsupport.BuiltInTool(t, "shell"), testsupport.ToolFingerprint(testsupport.BuiltInTool(t, "shell")), approval.Subject{Type: approval.SubjectAll}, approval.Allow); !errors.Is(err, approval.ErrInvalidRule) {
		t.Fatalf("NewRule error = %v, want ErrInvalidRule", err)
	}
	if _, _, err := approval.Decide(nil, approval.Query{SessionID: "ses_\u200bhidden", Tool: testsupport.BuiltInTool(t, "shell"), SourceFingerprint: testsupport.ToolFingerprint(testsupport.BuiltInTool(t, "shell"))}); !errors.Is(err, approval.ErrInvalidQuery) {
		t.Fatalf("Decide error = %v, want ErrInvalidQuery", err)
	}
	if _, err := (approval.RememberRequest{Subject: approval.Subject{Type: approval.SubjectAll},
		Scope: approval.ScopeSession, SessionID: " ses_1", Tool: testsupport.BuiltInTool(t, "shell"), Decision: approval.Allow, SourceFingerprint: testsupport.ToolFingerprint(testsupport.BuiltInTool(t, "shell")),
	}).Rule(); !errors.Is(err, approval.ErrInvalidRule) {
		t.Fatalf("RememberRequest.Rule error = %v, want ErrInvalidRule", err)
	}
}

func TestSessionModeValidation(t *testing.T) {
	tests := []struct {
		name  string
		state approval.SessionMode
		valid bool
	}{
		{name: "Plan restores safe", state: approval.SessionMode{Mode: approval.ModePlan, RestoreMode: approval.ModeSafe}, valid: true},
		{name: "Plan restores balanced", state: approval.SessionMode{Mode: approval.ModePlan, RestoreMode: approval.ModeBalanced}, valid: true},
		{name: "explicit yolo", state: approval.SessionMode{Mode: approval.ModeYolo}, valid: true},
		{name: "Plan cannot restore Plan", state: approval.SessionMode{Mode: approval.ModePlan, RestoreMode: approval.ModePlan}},
		{name: "unknown mode", state: approval.SessionMode{Mode: approval.Mode("invalid")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.state.Validate()
			if test.valid && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if !test.valid && !errors.Is(err, approval.ErrInvalidSessionMode) {
				t.Fatalf("Validate error = %v, want ErrInvalidSessionMode", err)
			}
		})
	}
}

func TestRuleValidationRejectsCorruptDurableValues(t *testing.T) {
	valid := mustRule(t, approval.ScopeProject, "/repo", "shell", approval.Subject{Type: approval.SubjectGlob, Value: "npm run *"}, approval.Allow)
	tests := []struct {
		name   string
		mutate func(*approval.Rule)
	}{
		{name: "identity drift", mutate: func(rule *approval.Rule) { rule.Tool = testsupport.BuiltInTool(t, "edit") }},
		{name: "unknown scope", mutate: func(rule *approval.Rule) { rule.Scope = approval.Scope("team") }},
		{name: "missing scope key", mutate: func(rule *approval.Rule) { rule.ScopeKey = "" }},
		{name: "unknown decision", mutate: func(rule *approval.Rule) { rule.Decision = approval.Decision("maybe") }},
		{name: "invalid glob", mutate: func(rule *approval.Rule) { rule.Subject.Value = "[" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rule := valid
			test.mutate(&rule)
			if err := rule.Validate(); !errors.Is(err, approval.ErrInvalidRule) {
				t.Fatalf("Validate error = %v, want ErrInvalidRule", err)
			}
		})
	}
}

func mustRule(t *testing.T, scope approval.Scope, scopeKey, toolName string, subject approval.Subject, decision approval.Decision) approval.Rule {
	t.Helper()
	rule, err := approval.NewRule(scope, scopeKey, testsupport.BuiltInTool(t, toolName), testsupport.ToolFingerprint(testsupport.BuiltInTool(t, toolName)), subject, decision)
	if err != nil {
		t.Fatalf("NewRule: %v", err)
	}
	return rule
}
