package toolset

import "testing"

func TestResolverForgetsOnlyRequestedSessionHistory(t *testing.T) {
	path := "/workspace/foo.go"
	content := contentFingerprint{3}
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
	content := contentFingerprint{3}
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
