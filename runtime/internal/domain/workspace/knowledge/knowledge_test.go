package knowledge

import (
	"errors"
	"strings"
	"testing"
)

func TestScopeIsAClosedVocabulary(t *testing.T) {
	for _, scope := range []Scope{ScopeCWD, ScopeProjectRoot, ScopeHome} {
		if err := scope.Validate(); err != nil {
			t.Fatalf("Validate(%q): %v", scope, err)
		}
		if scope.String() != string(scope) {
			t.Fatalf("String(%q) = %q", scope, scope.String())
		}
	}
	for _, scope := range []Scope{"", "workspace", "project", "user"} {
		if err := scope.Validate(); err == nil {
			t.Fatalf("Validate(%q) succeeded", scope)
		}
	}
}

func TestKnowledgeDocumentEnvelope(t *testing.T) {
	if err := ValidateDocument(strings.Repeat("x", int(MaxDocumentBytes))); err != nil {
		t.Fatalf("exact document boundary: %v", err)
	}
	if err := ValidateDocument(strings.Repeat("x", int(MaxDocumentBytes)+1)); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("oversized document error = %v, want ErrDocumentTooLarge", err)
	}
}

func TestEntryValidation(t *testing.T) {
	valid := Entry{Scope: ScopeHome, Path: "/home/FLAME.md", Revision: "rev-1"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid entry: %v", err)
	}

	tests := map[string]Entry{
		"scope":    {Scope: Scope("other"), Path: valid.Path, Revision: valid.Revision},
		"path":     {Scope: valid.Scope, Revision: valid.Revision},
		"revision": {Scope: valid.Scope, Path: valid.Path},
		"content": {
			Scope: valid.Scope, Path: valid.Path, Revision: valid.Revision,
			Content: strings.Repeat("x", int(MaxDocumentBytes)+1),
		},
	}
	for name, entry := range tests {
		t.Run(name, func(t *testing.T) {
			if err := entry.Validate(); err == nil {
				t.Fatal("invalid entry was accepted")
			}
		})
	}
}
