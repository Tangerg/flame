package knowledge

import (
	"errors"
	"strings"
	"testing"
)

func TestReplacementBindsOneValidatedKnowledgeCAS(t *testing.T) {
	replacement, err := NewReplacement(ScopeHome, "sha256:before", "new document")
	if err != nil {
		t.Fatalf("NewReplacement: %v", err)
	}
	if replacement.Scope() != ScopeHome || replacement.ExpectedRevision() != "sha256:before" ||
		replacement.Content() != "new document" {
		t.Fatalf("Replacement = %+v", replacement)
	}
}

func TestReplacementRejectsInvalidCommands(t *testing.T) {
	if _, err := NewReplacement(Scope("unknown"), "sha256:before", "content"); err == nil {
		t.Fatal("NewReplacement accepted an unknown scope")
	}
	if _, err := NewReplacement(ScopeHome, "", "content"); !errors.Is(err, ErrRevisionRequired) {
		t.Fatalf("missing revision error = %v, want ErrRevisionRequired", err)
	}
	if _, err := NewReplacement(ScopeHome, "sha256:before", strings.Repeat("x", int(MaxDocumentBytes)+1)); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("oversized content error = %v, want ErrDocumentTooLarge", err)
	}
}
