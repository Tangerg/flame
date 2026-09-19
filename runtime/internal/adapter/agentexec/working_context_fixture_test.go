package agentexec

import (
	"context"
	"testing"

	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/knowledge"
)

func mustWorkingContextComposer(t testing.TB, cfg WorkingContextConfig) *WorkingContextComposer {
	t.Helper()
	composer, err := NewWorkingContextComposer(workingContextTestConfig(cfg))
	if err != nil {
		t.Fatal(err)
	}
	return composer
}

func workingContextTestConfig(cfg WorkingContextConfig) WorkingContextConfig {
	empty := emptyWorkingContext{}
	if cfg.Knowledge == nil {
		cfg.Knowledge = empty
	}
	if cfg.AgentMemory == nil {
		cfg.AgentMemory = empty
	}
	if cfg.AgentMemorySearch == nil {
		cfg.AgentMemorySearch = empty
	}
	if cfg.Plan == nil {
		cfg.Plan = empty
	}
	if cfg.Goal == nil {
		cfg.Goal = empty
	}
	if cfg.Hooks == nil {
		cfg.Hooks = empty
	}
	return cfg
}

type emptyWorkingContext struct{}

func (emptyWorkingContext) Entries(context.Context, string) ([]knowledge.Entry, error) {
	return nil, nil
}
func (emptyWorkingContext) Items(context.Context, agentmemory.Scope, string) ([]agentmemory.Item, error) {
	return nil, nil
}
func (emptyWorkingContext) Search(context.Context, string, string, int) ([]agentmemory.Item, error) {
	return nil, nil
}
func (emptyWorkingContext) List(context.Context, string) ([]plan.Step, error) {
	return nil, nil
}
func (emptyWorkingContext) Current(context.Context, string) (goal.Goal, bool, error) {
	return goal.Goal{}, false, nil
}
func (emptyWorkingContext) For(context.Context, string) (*apphooks.Bound, error) {
	return apphooks.NewBound(nil, nil), nil
}

func TestWorkingContextRequiresCompleteSources(t *testing.T) {
	for name, remove := range map[string]func(*WorkingContextConfig){
		"knowledge": func(cfg *WorkingContextConfig) { cfg.Knowledge = nil },
		"plan":      func(cfg *WorkingContextConfig) { cfg.Plan = nil },
		"goal":      func(cfg *WorkingContextConfig) { cfg.Goal = nil },
		"hooks":     func(cfg *WorkingContextConfig) { cfg.Hooks = nil },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := workingContextTestConfig(WorkingContextConfig{})
			remove(&cfg)
			if composer, err := NewWorkingContextComposer(cfg); err == nil || composer != nil {
				t.Fatalf("incomplete context = %v, %v", composer, err)
			}
		})
	}
}
