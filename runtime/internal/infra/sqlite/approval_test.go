package sqlite_test

import (
	"context"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func newApprovalStore(t *testing.T) *sqlite.ApprovalRuleStore {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewApprovalRuleStore(db)
}

// TestApprovalRuleStore_VisibleScopes verifies the WHERE-clause scope predicate:
// a session rule, a project rule (keyed by dir), and a global rule are each
// visible only from the right session/dir, and global is visible everywhere.
func TestApprovalRuleStore_VisibleScopes(t *testing.T) {
	ctx := context.Background()
	store := newApprovalStore(t)

	put := func(r approval.Rule) {
		if err := store.Put(ctx, r); err != nil {
			t.Fatalf("put: %v", err)
		}
	}
	sessionRule := newApprovalRule(t, approval.ScopeSession, "sess1", "shell", "", approval.Allow)
	projectRule := newApprovalRule(t, approval.ScopeProject, "/proj/a", "edit", "", approval.Deny)
	globalRule := newApprovalRule(t, approval.ScopeGlobal, "", "read", "", approval.Allow)
	put(sessionRule)
	put(projectRule)
	put(globalRule)

	ids := func(sessionID, dir string) map[string]bool {
		rules, err := store.Visible(ctx, sessionID, dir, approval.MaximumVisibleRules+1)
		if err != nil {
			t.Fatalf("visible: %v", err)
		}
		m := map[string]bool{}
		for _, r := range rules {
			m[r.ID] = true
		}
		return m
	}

	// From sess1 in /proj/a: all three visible.
	if got := ids("sess1", "/proj/a"); !got[sessionRule.ID] || !got[projectRule.ID] || !got[globalRule.ID] || len(got) != 3 {
		t.Fatalf("sess1@/proj/a sees %v, want s+p+g", got)
	}
	// From another session in another dir: only global.
	if got := ids("sess2", "/proj/b"); got[sessionRule.ID] || got[projectRule.ID] || !got[globalRule.ID] || len(got) != 1 {
		t.Fatalf("sess2@/proj/b sees %v, want only g", got)
	}
	// With no cwd: project rule must not match (skipped when dir is empty).
	if got := ids("sess1", ""); got[projectRule.ID] {
		t.Fatalf("project rule leaked with empty dir: %v", got)
	}
}

func TestApprovalRuleStoreReplacesDecisionAndAuthority(t *testing.T) {
	ctx := t.Context()
	store := newApprovalStore(t)
	rule, err := approval.NewRule(approval.ScopeGlobal, "", testsupport.A2ATool(t, "remote"), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", approval.Subject{Type: approval.SubjectAll}, approval.Deny)
	if err != nil {
		t.Fatal(err)
	}
	id := rule.ID
	for _, decision := range []approval.Decision{approval.Deny, approval.Allow, approval.Deny} {
		fingerprint := rule.SourceFingerprint
		if decision == approval.Allow {
			fingerprint = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		}
		next, err := approval.NewRule(rule.Scope, rule.ScopeKey, rule.Tool, fingerprint, rule.Subject, decision)
		if err != nil {
			t.Fatal(err)
		}
		if next.ID != id {
			t.Fatal("changing a decision changed the rule identity")
		}
		if err := store.Put(ctx, next); err != nil {
			t.Fatal(err)
		}
		rules, err := store.Visible(ctx, "s", "/p", approval.MaximumVisibleRules+1)
		if err != nil || len(rules) != 1 || rules[0] != next {
			t.Fatalf("rules after %s = %v, %v; want [%v]", decision, rules, err, next)
		}
		if got, found, err := approval.Decide(rules, approval.Query{Tool: next.Tool, SourceFingerprint: fingerprint}); err != nil || !found || got != decision {
			t.Fatalf("decision = %v, %v, %v; want %s", got, found, err, decision)
		}
	}
	if err := store.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	rules, err := store.Visible(ctx, "s", "/p", approval.MaximumVisibleRules+1)
	if err != nil || len(rules) != 0 {
		t.Fatalf("after delete = %v, %v", rules, err)
	}
}

func TestApprovalSubjectTypesSurviveRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "flame.db")
	db, err := sqlite.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewApprovalRuleStore(db)
	ref := testsupport.BuiltInTool(t, "shell")
	for _, subject := range []approval.Subject{
		{Type: approval.SubjectGlob, Value: "echo *"},
		approval.InvocationSubject("echo *"),
		approval.InvocationSubject("echo ["),
	} {
		decision := approval.Deny
		if subject.Type == approval.SubjectExact {
			decision = approval.Allow
		}
		rule, err := approval.NewRule(approval.ScopeGlobal, "", ref, "", subject, decision)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Put(t.Context(), rule); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := sqlite.NewApprovalRuleStore(db).Visible(t.Context(), "", "", approval.MaximumVisibleRules+1)
	if err != nil || len(rules) != 3 {
		t.Fatalf("restored rules = %+v, %v; want three distinct matchers", rules, err)
	}
	for _, test := range []struct {
		subject string
		want    approval.Decision
	}{
		{"echo *", approval.Allow},
		{"echo [", approval.Allow},
		{"echo something else", approval.Deny},
	} {
		got, found, err := approval.Decide(rules, approval.Query{Tool: ref, Subject: test.subject})
		if err != nil || !found || got != test.want {
			t.Fatalf("subject %q: %s, %t, %v; want %s", test.subject, got, found, err, test.want)
		}
	}
}

func TestApprovalRuleStore_DeleteSessionPreservesBroaderScopes(t *testing.T) {
	ctx := context.Background()
	store := newApprovalStore(t)
	sessionOne := newApprovalRule(t, approval.ScopeSession, "sess1", "shell", "", approval.Allow)
	sessionTwo := newApprovalRule(t, approval.ScopeSession, "sess2", "shell", "", approval.Allow)
	project := newApprovalRule(t, approval.ScopeProject, "/proj", "edit", "", approval.Allow)
	global := newApprovalRule(t, approval.ScopeGlobal, "", "read", "", approval.Allow)
	for _, rule := range []approval.Rule{sessionOne, sessionTwo, project, global} {
		if err := store.Put(ctx, rule); err != nil {
			t.Fatalf("put %s: %v", rule.ID, err)
		}
	}
	if err := store.DeleteSession(ctx, "sess1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	rules, err := store.Visible(ctx, "sess1", "/proj", approval.MaximumVisibleRules+1)
	if err != nil {
		t.Fatalf("Visible: %v", err)
	}
	ids := map[string]bool{}
	for _, rule := range rules {
		ids[rule.ID] = true
	}
	if ids[sessionOne.ID] || !ids[project.ID] || !ids[global.ID] || len(ids) != 2 {
		t.Fatalf("visible after DeleteSession = %v, want p+g", ids)
	}
	if rules, err := store.Visible(ctx, "sess2", "", approval.MaximumVisibleRules+1); err != nil || len(rules) != 2 {
		t.Fatalf("other session after DeleteSession = %+v, %v, want s2+g", rules, err)
	}
}

func TestApprovalRuleStore_VisibleEnforcesRequestedBound(t *testing.T) {
	ctx := t.Context()
	store := newApprovalStore(t)
	for _, toolName := range []string{"read", "shell"} {
		if err := store.Put(ctx, newApprovalRule(t, approval.ScopeGlobal, "", toolName, "", approval.Allow)); err != nil {
			t.Fatal(err)
		}
	}
	rules, err := store.Visible(ctx, "s", "/repo", 1)
	if err != nil || len(rules) != 1 {
		t.Fatalf("Visible limit = %+v, %v; want one row", rules, err)
	}
	for _, limit := range []int{0, approval.MaximumVisibleRules + 2} {
		if _, err := store.Visible(ctx, "s", "/repo", limit); err == nil {
			t.Fatalf("Visible accepted invalid limit %d", limit)
		}
	}
}

func newApprovalRule(t *testing.T, scope approval.Scope, scopeKey, toolName, subject string, decision approval.Decision) approval.Rule {
	t.Helper()
	rule, err := approval.NewRule(scope, scopeKey, testsupport.BuiltInTool(t, toolName), testsupport.ToolFingerprint(testsupport.BuiltInTool(t, toolName)), approval.InvocationSubject(subject), decision)
	if err != nil {
		t.Fatalf("new approval rule: %v", err)
	}
	return rule
}
