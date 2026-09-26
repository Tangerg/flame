package agentexec

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/workspace"
)

func TestAgentDocumentsPromptAnnotatesEachSource(t *testing.T) {
	out := agentDocumentsPromptForTest(t, []workspace.AgentDocFile{
		{Path: "/a/AGENTS.md", Content: "alpha", Scope: workspace.AgentDocScopeProjectRoot},
		{Path: "/b/AGENTS.md", Content: "beta", Scope: workspace.AgentDocScopeCWD},
	}, agentDocPromptMaxBytes).text

	for _, want := range []string{"<!-- From: /a/AGENTS.md -->", "<!-- From: /b/AGENTS.md -->", "alpha", "beta"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered docs = %q, missing %q", out, want)
		}
	}
	if strings.Index(out, "alpha") > strings.Index(out, "beta") {
		t.Fatalf("docs are out of order: %q", out)
	}
}

func TestAgentDocumentsPromptKeepsCompleteCascadeAtExactBudget(t *testing.T) {
	files := []workspace.AgentDocFile{
		{Path: "/root/AGENTS.md", Content: strings.Repeat("a", 100), Scope: workspace.AgentDocScopeProjectRoot},
		{Path: "/leaf/AGENTS.md", Content: "leaf", Scope: workspace.AgentDocScopeCWD},
	}
	expected := agentDocPromptHeader + "\n\n<!-- From: /root/AGENTS.md -->\n" + strings.Repeat("a", 100) + "\n\n<!-- From: /leaf/AGENTS.md -->\nleaf\n"
	prompt := agentDocumentsPromptForTest(t, files, len(expected))
	if got := agentDocPromptHeader + "\n\n" + prompt.text; got != expected {
		t.Fatalf("complete cascade = %q, want %q", got, expected)
	}
	if len(prompt.sources) != 2 || prompt.sources[0].Reference != "/root/AGENTS.md" || prompt.sources[1].Reference != "/leaf/AGENTS.md" {
		t.Fatalf("projected sources = %v", prompt.sources)
	}
	if agentDocumentsPromptForTest(t, nil, 0).text != "" {
		t.Fatal("empty input must render no prompt text")
	}
}

func TestAgentDocumentsPromptRejectsOverflowWithoutDroppingAncestors(t *testing.T) {
	files := []workspace.AgentDocFile{
		{Path: "/home/AGENTS.md", Content: strings.Repeat("r", agentDocPromptMaxBytes/2), Scope: workspace.AgentDocScopeHome},
		{Path: "/root/AGENTS.md", Content: strings.Repeat("n", agentDocPromptMaxBytes/2), Scope: workspace.AgentDocScopeProjectRoot},
		{Path: "/leaf/AGENTS.md", Content: "leaf", Scope: workspace.AgentDocScopeCWD},
	}
	for _, budget := range []int{agentDocPromptMaxBytes, 0} {
		prompt, err := newAgentDocumentsPrompt(files, budget)
		if !errors.Is(err, workspace.ErrPromptSourceTooLarge) {
			t.Fatalf("budget %d error = %v, want explicit overflow", budget, err)
		}
		if prompt.text != "" || len(prompt.sources) != 0 {
			t.Fatalf("overflow returned a partial prompt: %+v", prompt)
		}
	}
}

func agentDocumentsPromptForTest(
	t *testing.T,
	files []workspace.AgentDocFile,
	maxBytes int,
) agentDocumentsPrompt {
	t.Helper()
	prompt, err := newAgentDocumentsPrompt(files, maxBytes)
	if err != nil {
		t.Fatal(err)
	}
	return prompt
}
