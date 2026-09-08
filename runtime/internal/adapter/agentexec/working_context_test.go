package agentexec

import (
	"context"
	"testing"

	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

func testWorkingContextConfig(t *testing.T, config WorkingContextConfig) WorkingContextConfig {
	t.Helper()
	if config.UserHome == "" {
		config.UserHome = t.TempDir()
	}
	if config.Knowledge == nil {
		config.Knowledge = &stubKnowledgeStore{}
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
		{"knowledge", func(c *WorkingContextConfig) { c.Knowledge = nil }},
		{"typed nil knowledge", func(c *WorkingContextConfig) { c.Knowledge = (*stubKnowledgeStore)(nil) }},
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
