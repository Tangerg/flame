package workspace

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestAuthoredPromptDocumentContract(t *testing.T) {
	exact := []byte(strings.Repeat("a", MaxAuthoredPromptDocumentBytes))
	if err := ValidateAuthoredPromptDocument(exact); err != nil {
		t.Fatalf("exact boundary error = %v", err)
	}
	if err := ValidateAuthoredPromptDocument(append(exact, 'a')); !errors.Is(err, ErrPromptSourceTooLarge) {
		t.Fatalf("oversized error = %v, want ErrPromptSourceTooLarge", err)
	}
	if err := ValidateAuthoredPromptDocument([]byte{'a', 0xff}); !errors.Is(err, ErrInvalidPromptSource) {
		t.Fatalf("invalid UTF-8 error = %v, want ErrInvalidPromptSource", err)
	}
}

func TestAgentDocumentCascadeContract(t *testing.T) {
	document := strings.Repeat("a", MaxAuthoredPromptDocumentBytes)
	exact := make([]AgentDocFile, MaxAgentDocumentCascadeBytes/MaxAuthoredPromptDocumentBytes)
	for index := range exact {
		exact[index] = AgentDocFile{
			Path: fmt.Sprintf("/repo/%d/AGENTS.md", index), Content: document,
			Scope: AgentDocScopeProjectRoot,
		}
	}
	if err := ValidateAgentDocumentCascade(exact); err != nil {
		t.Fatalf("exact aggregate boundary error = %v", err)
	}
	if err := ValidateAgentDocumentCascade(append(exact, AgentDocFile{
		Path: "/repo/leaf/AGENTS.md", Content: "x", Scope: AgentDocScopeCWD,
	})); !errors.Is(err, ErrPromptSourceTooLarge) {
		t.Fatalf("aggregate overflow error = %v, want ErrPromptSourceTooLarge", err)
	}

	overfull := make([]AgentDocFile, MaxAgentDocumentsPerCascade+1)
	for index := range overfull {
		overfull[index] = AgentDocFile{
			Path: fmt.Sprintf("/repo/%d/AGENTS.md", index), Content: "x",
			Scope: AgentDocScopeProjectRoot,
		}
	}
	if err := ValidateAgentDocumentCascade(overfull); !errors.Is(err, ErrPromptSourceTooLarge) {
		t.Fatalf("overfull error = %v, want ErrPromptSourceTooLarge", err)
	}

	for _, invalid := range [][]AgentDocFile{
		{
			{Path: "/repo/AGENTS.md", Content: "root", Scope: AgentDocScopeProjectRoot},
			{Path: "/repo/AGENTS.md", Content: "leaf", Scope: AgentDocScopeCWD},
		}, {
			{Path: "/repo/AGENTS.md", Content: "project", Scope: AgentDocScopeProjectRoot},
			{Path: "/home/AGENTS.md", Content: "home", Scope: AgentDocScopeHome},
		},
	} {
		if err := ValidateAgentDocumentCascade(invalid); !errors.Is(err, ErrInvalidPromptSource) {
			t.Errorf("invalid cascade error = %v, want ErrInvalidPromptSource", err)
		}
	}
}
