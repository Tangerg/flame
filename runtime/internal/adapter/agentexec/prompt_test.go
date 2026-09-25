package agentexec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
)

// TestComposeSystemPrompt_BaseOnly verifies empty context sources yield the
// base prompt without resource headers.
func TestComposeSystemPrompt_BaseOnly(t *testing.T) {
	got := composeSystemPromptText(t, WorkingContextConfig{}, "")
	if !strings.Contains(got, "You are Flame") {
		t.Errorf("base prompt missing identity, got %q", got)
	}
	if strings.Contains(got, "## Pinned memory") || strings.Contains(got, agentDocPromptHeader) {
		t.Error("empty sources should not produce section headers")
	}
}

// TestComposePromptPlacesCuratedMemoryAboveAuthoredInstructions pins the
// remaining precedence contract: agent-curated memory is context the authored
// AGENTS.md cascade may override, so it has to be read first.
func TestComposePromptPlacesCuratedMemoryAboveAuthoredInstructions(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("repository convention"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := composeSystemPromptText(t, WorkingContextConfig{
		AgentMemory: stubAgentMemory{content: "agent learned fact"},
	}, workspace)

	curatedIndex := strings.Index(got, "## Pinned memory")
	authoredIndex := strings.Index(got, agentDocPromptHeader)
	if curatedIndex < 0 || authoredIndex < 0 || curatedIndex > authoredIndex {
		t.Fatalf("prompt precedence is wrong:\n%s", got)
	}
}

func TestComposePromptUsesInjectedUserHomeForAgentDocs(t *testing.T) {
	userHome := t.TempDir()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(userHome, ".flame"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userHome, ".flame", "AGENTS.md"), []byte("injected home rule"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := composeSystemPromptText(t, WorkingContextConfig{UserHome: userHome}, workspace)
	if !strings.Contains(got, "injected home rule") {
		t.Fatalf("prompt did not use injected user home:\n%s", got)
	}
}

// TestComposePromptPreservesTheAgentDocumentCascade — the authored cascade is
// read broadest to narrowest so a workspace document extends, rather than
// races, the project one.
func TestComposePromptPreservesTheAgentDocumentCascade(t *testing.T) {
	projectRoot := t.TempDir()
	workspace := filepath.Join(projectRoot, "packages", "app")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	// Discovery anchors the cascade at the nearest ancestor holding `.git`;
	// without one the scan is single-level and the project document is invisible.
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "AGENTS.md"), []byte("repository convention"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("workspace override"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := composeSystemPromptText(t, WorkingContextConfig{}, workspace)
	project := strings.Index(got, "repository convention")
	local := strings.Index(got, "workspace override")
	if project < 0 || local <= project {
		t.Fatalf("agent document cascade precedence is wrong:\n%s", got)
	}
}

func TestComposePromptRejectsSilentlyDroppedAgentDocument(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspace, "AGENTS.md"),
		[]byte(strings.Repeat("r", agentDocPromptMaxBytes+1)),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := newTestWorkingContextComposer(t, WorkingContextConfig{}).
		composeSystemMessage(t.Context(), workspace); !errors.Is(err, workspaceapp.ErrPromptSourceTooLarge) {
		t.Fatalf("composeSystemMessage error = %v, want ErrPromptSourceTooLarge", err)
	}
}

// ------------------------------------------------------------------
// helpers
// ------------------------------------------------------------------

func composeSystemPromptText(t *testing.T, config WorkingContextConfig, cwd string) string {
	t.Helper()
	message, err := newTestWorkingContextComposer(t, config).composeSystemMessage(t.Context(), cwd)
	if err != nil {
		t.Fatal(err)
	}
	return message.Text()
}

type stubAgentMemory struct{ content string }

func (s stubAgentMemory) Items(_ context.Context, scope agentmemory.Scope, _ string) ([]agentmemory.Item, error) {
	if scope != agentmemory.ScopeProject || strings.TrimSpace(s.content) == "" {
		return nil, nil
	}
	// Pinned so it reaches the always-on core (the composer injects pinned only).
	return []agentmemory.Item{{Content: s.content, Pinned: true, Status: agentmemory.StatusActive}}, nil
}
