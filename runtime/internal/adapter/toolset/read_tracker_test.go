package toolset

import "testing"

// TestTracker drives the pure read-before-mutation invariant directly: a path must
// be recorded before it passes Check, a changed fingerprint is stale, and
// Refresh permits consecutive mutations.
func TestTracker(t *testing.T) {
	path := "/workspace/foo.go"
	one := fingerprintOf([]byte("one"))
	two := fingerprintOf([]byte("two"))
	tr := newReadTracker()
	const sess = "s1"

	// Never read → a structured read-required verdict.
	if got := tr.check(sess, path, one); got != readRequired || got.allowed() {
		t.Fatalf("unread Check = %v, want missing", got)
	}

	// Read (full) → passes; a session boundary is respected.
	tr.record(sess, path, one)
	if got := tr.check(sess, path, one); got != mutationAllowed || !got.allowed() {
		t.Fatalf("read Check = %v, want ok", got)
	}
	if got := tr.check("other", path, one); got != readRequired {
		t.Fatalf("cross-session Check = %v, want missing (per-session isolation)", got)
	}

	// Changed content → stale.
	if got := tr.check(sess, path, two); got != contentChanged {
		t.Fatalf("changed Check = %v, want stale", got)
	}

	// Refresh re-stamps the current content → passes again.
	tr.refresh(sess, path, two)
	if got := tr.check(sess, path, two); got != mutationAllowed {
		t.Fatalf("post-refresh Check = %v, want ok", got)
	}
}

func TestResolverForgetsOnlyRequestedSessionHistory(t *testing.T) {
	path := "/workspace/foo.go"
	content := fingerprintOf([]byte("content"))
	tracker := newReadTracker()
	tracker.record("restored", path, content)
	tracker.record("untouched", path, content)
	resolver := &Resolver{readTracker: tracker}

	resolver.ForgetSessionContext("restored")

	if got := tracker.check("restored", path, content); got != readRequired {
		t.Fatalf("restored Session check = %v, want fresh read required", got)
	}
	if got := tracker.check("untouched", path, content); got != mutationAllowed {
		t.Fatalf("untouched Session check = %v, want existing authority retained", got)
	}
}

func TestResolverForgetsWorkspaceReadsAcrossSessions(t *testing.T) {
	content := fingerprintOf([]byte("content"))
	tracker := newReadTracker()
	tracker.record("owner", "/workspace/a.go", content)
	tracker.record("sibling", "/workspace/nested/b.go", content)
	tracker.record("outside", "/workspace-other/c.go", content)
	resolver := &Resolver{readTracker: tracker}

	resolver.ForgetWorkspace("/workspace")

	for _, sessionPath := range [][2]string{
		{"owner", "/workspace/a.go"},
		{"sibling", "/workspace/nested/b.go"},
	} {
		if got := tracker.check(sessionPath[0], sessionPath[1], content); got != readRequired {
			t.Fatalf("check(%q, %q) = %v, want fresh read required", sessionPath[0], sessionPath[1], got)
		}
	}
	if got := tracker.check("outside", "/workspace-other/c.go", content); got != mutationAllowed {
		t.Fatalf("outside workspace check = %v, want existing authority retained", got)
	}
}
