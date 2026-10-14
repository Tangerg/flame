package execution

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	agentmemory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	corechat "github.com/Tangerg/scope/core/chat"
)

func testWorkingContextConfig(t *testing.T, config WorkingContextConfig) WorkingContextConfig {
	t.Helper()
	if config.UserHome == "" {
		config.UserHome = t.TempDir()
	}
	if config.AgentMemory == nil {
		config.AgentMemory = provenanceMemoryReader{}
	}
	if config.AgentMemorySearch == nil {
		config.AgentMemorySearch = &fakeAgentMemorySearcher{}
	}
	if config.Plan == nil {
		config.Plan = emptyContextPlan{}
	}
	if config.Goal == nil {
		config.Goal = emptyContextGoal{}
	}
	if config.Hooks == nil {
		config.Hooks = provenanceHookResolver{bound: apphooks.NewBound(nil, nil)}
	}
	return config
}

func TestWorkingContextRecallPreservesConversationAndOpeningInput(t *testing.T) {
	seed := []corechat.Message{
		corechat.NewUserMessage(corechat.NewTextPart("previous question")),
		corechat.NewAssistantMessage(corechat.NewTextPart("previous answer")),
		corechat.NewUserMessage(corechat.NewTextPart("current question"), corechat.NewTextPart("current attachment")),
	}
	original := cloneChatMessages(seed)
	search := &fakeAgentMemorySearcher{items: []MemoryPromptItem{
		testMemoryPromptItem(agentmemory.ItemSnapshot{ID: testAgentMemoryItemID(t, '1'), Content: "relevant fact"}),
	}}
	composer := newTestWorkingContextComposer(t, WorkingContextConfig{AgentMemorySearch: search})
	cwd := t.TempDir()
	messages, err := composer.ComposeWorkingContext(t.Context(), runs.WorkingContextInput{
		SessionID: "session:recall", CWD: cwd, WorkspaceCWD: cwd, PromptText: "current question", Seed: seed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != len(seed)+1 {
		t.Fatalf("context has %d messages, want instructions plus the %d seeded messages", len(messages), len(seed))
	}
	if !reflect.DeepEqual(seed, original) || !reflect.DeepEqual(messages[1:len(messages)-1], original[:len(original)-1]) {
		t.Fatal("recall changed its input or the historical conversation")
	}
	opening := messages[len(messages)-1]
	if opening.Role != corechat.RoleUser || len(opening.Parts) != 3 || !reflect.DeepEqual(opening.Parts[:2], original[len(original)-1].Parts) || !strings.Contains(opening.Parts[2].Text, "relevant fact") {
		t.Fatalf("opening is not the exact user prompt followed by recalled data: %+v", opening)
	}
}

func newTestWorkingContextComposer(t *testing.T, config WorkingContextConfig) *WorkingContextComposer {
	t.Helper()
	composer, err := NewWorkingContextComposer(testWorkingContextConfig(t, config))
	if err != nil {
		t.Fatal(err)
	}
	return composer
}

type emptyContextPlan struct{}

func (emptyContextPlan) List(context.Context, string) ([]plan.Step, error) { return nil, nil }

type emptyContextGoal struct{}

func (emptyContextGoal) Current(context.Context, string) (goal.Goal, bool, error) {
	return goal.Goal{}, false, nil
}

func TestNewWorkingContextComposerRequiresCompleteDependencies(t *testing.T) {
	for _, test := range []struct {
		name   string
		remove func(*WorkingContextConfig)
	}{
		{"user home", func(c *WorkingContextConfig) { c.UserHome = "" }},
		{"relative user home", func(c *WorkingContextConfig) { c.UserHome = "relative" }},
		{"memory", func(c *WorkingContextConfig) { c.AgentMemory = nil }},
		{"typed nil memory", func(c *WorkingContextConfig) { c.AgentMemory = (*provenanceMemoryReader)(nil) }},
		{"memory search", func(c *WorkingContextConfig) { c.AgentMemorySearch = nil }},
		{"typed nil memory search", func(c *WorkingContextConfig) { c.AgentMemorySearch = (*fakeAgentMemorySearcher)(nil) }},
		{"plan", func(c *WorkingContextConfig) { c.Plan = nil }},
		{"typed nil plan", func(c *WorkingContextConfig) { c.Plan = (*emptyContextPlan)(nil) }},
		{"goal", func(c *WorkingContextConfig) { c.Goal = nil }},
		{"typed nil goal", func(c *WorkingContextConfig) { c.Goal = (*emptyContextGoal)(nil) }},
		{"hooks", func(c *WorkingContextConfig) { c.Hooks = nil }},
		{"typed nil hooks", func(c *WorkingContextConfig) { c.Hooks = (*provenanceHookResolver)(nil) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := testWorkingContextConfig(t, WorkingContextConfig{})
			test.remove(&config)
			if composer, err := NewWorkingContextComposer(config); err == nil || composer != nil {
				t.Fatalf("NewWorkingContextComposer = (%v, %v), want incomplete construction rejected", composer, err)
			}
		})
	}
}
