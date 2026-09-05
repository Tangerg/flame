package agentexec

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	domainhooks "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/metadata"
)

func TestSystemPromptProvenanceMatchesVisibleComposition(t *testing.T) {
	cwd := t.TempDir()
	if err := os.Mkdir(filepath.Join(cwd, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	document := filepath.Join(cwd, "AGENTS.md")
	if err := os.WriteFile(document, []byte("agent document"), 0o644); err != nil {
		t.Fatal(err)
	}
	canonicalDocument, canonicalErr := filepath.EvalSymlinks(document)
	if canonicalErr != nil {
		t.Fatal(canonicalErr)
	}
	knowledge := &stubKnowledgeStore{home: "user rule", cwd: "workspace rule"}
	pinnedMemoryID := testAgentMemoryItemID(t, '1')
	memory := provenanceMemoryReader{items: []agentmemory.Item{{
		ID: pinnedMemoryID, Content: "remember this", Pinned: true,
	}}}
	composer := NewWorkingContextComposer(WorkingContextConfig{
		Knowledge:   knowledge,
		AgentMemory: memory,
		Plan:        provenancePlanReader{},
	})
	message, composeErr := composer.composeSystemMessage(t.Context(), cwd)
	if composeErr != nil {
		t.Fatal(composeErr)
	}
	if err := message.Validate(); err != nil {
		t.Fatal(err)
	}

	provenance := decodeContextProvenance(t, message.Metadata)
	wantKinds := []contextSourceKind{
		contextSourceBasePrompt,
		contextSourceUserKnowledge,
		contextSourcePinnedMemory,
		contextSourceProjectKnowledge,
		contextSourceAgentDocument,
	}
	gotKinds := make([]contextSourceKind, len(provenance))
	for index, source := range provenance {
		gotKinds[index] = source.Kind
	}
	if !slices.Equal(gotKinds, wantKinds) {
		t.Fatalf("source kinds=%v, want %v", gotKinds, wantKinds)
	}
	if provenance[2].Reference != pinnedMemoryID.String() ||
		provenance[2].Purpose != contextPurposeData ||
		provenance[4].Reference != canonicalDocument {
		t.Fatalf("provenance=%+v", provenance)
	}
	currentState, stateErr := composer.CurrentSessionState(t.Context(), "session:one")
	if stateErr != nil || len(currentState) != 1 {
		t.Fatalf("CurrentSessionState messages=%d error=%v", len(currentState), stateErr)
	}
	planProvenance := decodeContextProvenance(t, currentState[0].Metadata)
	if len(planProvenance) != 1 ||
		planProvenance[0].Kind != contextSourceSessionPlan ||
		planProvenance[0].Reference != "session:one" ||
		planProvenance[0].Purpose != contextPurposeData {
		t.Fatalf("Plan provenance=%+v", planProvenance)
	}
}

func TestPromptCompositionRejectsSourcePurposeMismatch(t *testing.T) {
	var prompt promptComposition
	prompt.append(basePrompt, contextSource{
		Kind: contextSourceBasePrompt, Purpose: contextPurposeData,
	})
	if _, err := prompt.systemMessage(); err == nil {
		t.Fatal("source kind and purpose mismatch must be rejected")
	}
}

func TestWorkingContextAttributesHookAndRecalledMemoryInPlace(t *testing.T) {
	cwd := t.TempDir()
	hooks := []domainhooks.Hook{
		{Event: domainhooks.SessionStart, Inject: "session context"},
		{Event: domainhooks.UserPromptSubmit, Inject: "turn context"},
	}
	recalledMemoryID := testAgentMemoryItemID(t, '2')
	composer := NewWorkingContextComposer(WorkingContextConfig{
		Hooks: provenanceHookResolver{bound: apphooks.NewBound(hooks, apphooks.NewRunner(nil, nil))},
		AgentMemorySearch: &fakeAgentMemorySearcher{
			items: []agentmemory.Item{{ID: recalledMemoryID, Content: "recalled fact"}},
		},
	})
	messages, err := composer.ComposeWorkingContext(t.Context(), runs.WorkingContextInput{
		SessionID:  "session:one",
		CWD:        cwd,
		PromptText: "question",
		Seed: []corechat.Message{
			corechat.NewUserMessage(corechat.NewTextPart("question")),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 {
		t.Fatalf("messages=%d, want system + recall + user", len(messages))
	}
	for index, message := range messages {
		if err := message.Validate(); err != nil {
			t.Fatalf("message[%d]: %v", index, err)
		}
	}

	system := decodeContextProvenance(t, messages[0].Metadata)
	if len(system) != 1 || system[0].Kind != contextSourceBasePrompt {
		t.Fatalf("system provenance=%+v", system)
	}
	recalled := decodeContextProvenance(t, messages[1].Metadata)
	if len(recalled) != 1 || recalled[0].Kind != contextSourceRecalledMemory ||
		recalled[0].Reference != recalledMemoryID.String() ||
		recalled[0].Purpose != contextPurposeData {
		t.Fatalf("recall provenance=%+v", recalled)
	}
	hook := decodeContextProvenance(t, messages[2].Parts[0].Metadata)
	if len(hook) != 2 ||
		hook[0].Reference != string(domainhooks.SessionStart) ||
		hook[1].Reference != string(domainhooks.UserPromptSubmit) {
		t.Fatalf("hook provenance=%+v", hook)
	}
	if len(messages[2].Parts) != 2 || messages[2].Parts[1].Text != "question" {
		t.Fatalf("hook injection changed user part ordering: %+v", messages[2].Parts)
	}
}

func TestInteractionInstructionContextStopsBeforeDurableSummary(t *testing.T) {
	prompt, err := (promptComposition{sections: []promptSection{{
		text: "runtime instructions",
		sources: contextSources{
			contextSourceBasePrompt.source(""),
		},
	}}}).systemMessage()
	if err != nil {
		t.Fatal(err)
	}
	summary := corechat.NewSystemMessage("[Earlier conversation summary]\ncompleted work")
	messages := []corechat.Message{
		prompt,
		summary,
		corechat.NewUserMessage(corechat.NewTextPart("continue")),
	}

	instructions, err := interactionInstructionContext(messages)
	if err != nil {
		t.Fatal(err)
	}
	if len(instructions) != 1 || instructions[0].Text() != "runtime instructions" {
		t.Fatalf("instructions = %#v, want only provenance-owned prompt", instructions)
	}
}

func TestInteractionInstructionContextStopsBeforeReplaceableSessionPlan(t *testing.T) {
	composer := NewWorkingContextComposer(WorkingContextConfig{Plan: provenancePlanReader{}})
	instructions, err := composer.composeSystemMessage(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	currentState, err := composer.CurrentSessionState(t.Context(), "session:one")
	if err != nil || len(currentState) != 1 {
		t.Fatalf("CurrentSessionState messages=%d error=%v", len(currentState), err)
	}
	messages := []corechat.Message{
		instructions,
		currentState[0],
		corechat.NewUserMessage(corechat.NewTextPart("continue")),
	}

	frozen, err := interactionInstructionContext(messages)
	if err != nil {
		t.Fatal(err)
	}
	if len(frozen) != 1 || strings.Contains(frozen[0].Text(), "verify provenance") {
		t.Fatalf("frozen instructions = %#v, want no Session Plan", frozen)
	}
}

func decodeContextProvenance(t *testing.T, values metadata.Map) contextSources {
	t.Helper()
	provenance, found, err := values.Decode[contextSources](contextProvenanceMetadataKey)
	if err != nil || !found {
		t.Fatalf("decode context provenance found=%t error=%v", found, err)
	}
	if len(provenance) == 0 {
		t.Fatalf("context provenance=%+v", provenance)
	}
	return provenance
}

type provenanceMemoryReader struct {
	items []agentmemory.Item
}

func (p provenanceMemoryReader) Items(
	_ context.Context,
	scope agentmemory.Scope,
	_ string,
) ([]agentmemory.Item, error) {
	if scope != agentmemory.ScopeProject {
		return nil, nil
	}
	return slices.Clone(p.items), nil
}

type provenancePlanReader struct{}

func (provenancePlanReader) List(context.Context, string) ([]plan.Step, error) {
	return []plan.Step{{Description: "verify provenance", Status: plan.StatusInProgress}}, nil
}

type provenanceHookResolver struct {
	bound *apphooks.Bound
}

func (p provenanceHookResolver) For(context.Context, string) (*apphooks.Bound, error) {
	return p.bound, nil
}
