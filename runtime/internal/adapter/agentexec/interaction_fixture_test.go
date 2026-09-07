package agentexec

import (
	"context"
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func newInteractionTestExecutor(t testing.TB, cfg InteractionExecutorConfig) (*InteractionExecutor, error) {
	t.Helper()
	return NewInteractionExecutor(interactionTestCapabilities(t, cfg))
}

func interactionTestCapabilities(t testing.TB, cfg InteractionExecutorConfig) InteractionExecutorConfig {
	t.Helper()
	context := mustWorkingContextComposer(t, WorkingContextConfig{})
	if cfg.ToolResolver == nil {
		cfg.ToolResolver = staticInteractionTools{}
	}
	if cfg.ToolInterpreter == nil {
		cfg.ToolInterpreter = testInteractionToolInterpreter{}
	}
	if cfg.ToolPresenter == nil {
		cfg.ToolPresenter = testInteractionToolPresenter{}
	}
	if cfg.ToolAuthorizer == nil {
		cfg.ToolAuthorizer = allowInteractionTools{}
	}
	if cfg.ToolHooks == nil {
		cfg.ToolHooks = context
	}
	if cfg.MCPToolAutoApproved == nil {
		cfg.MCPToolAutoApproved = func(string, string) bool { return false }
	}
	if cfg.Maintenance == nil {
		cfg.Maintenance = fixedRunMaintenance{}
	}
	if cfg.ModelContextCompactor == nil {
		cfg.ModelContextCompactor = unchangedInteractionContext{}
	}
	if cfg.ModelContextState == nil {
		cfg.ModelContextState = emptyInteractionModelContextState{}
	}
	if cfg.LifecycleHooks == nil {
		cfg.LifecycleHooks = context
	}
	return cfg
}

type unchangedInteractionContext struct{}

func (unchangedInteractionContext) CompactModelContext(_ context.Context, request ModelContextCompaction) (ModelContextCompactionResult, error) {
	return NewModelContextCompactionResult(request.Candidate(), false, "", len(request.Candidate()), 100)
}

func TestInteractionRequiresCompleteCapabilities(t *testing.T) {
	executor := newTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return interactionTextResponse("unused"), nil
	}))
	for name, remove := range map[string]func(*InteractionExecutorConfig){
		"tool resolver":    func(cfg *InteractionExecutorConfig) { cfg.ToolResolver = nil },
		"tool interpreter": func(cfg *InteractionExecutorConfig) { cfg.ToolInterpreter = nil },
		"tool authorizer":  func(cfg *InteractionExecutorConfig) { cfg.ToolAuthorizer = nil },
		"tool hooks":       func(cfg *InteractionExecutorConfig) { cfg.ToolHooks = nil },
		"compactor":        func(cfg *InteractionExecutorConfig) { cfg.ModelContextCompactor = nil },
		"context state":    func(cfg *InteractionExecutorConfig) { cfg.ModelContextState = nil },
		"maintenance":      func(cfg *InteractionExecutorConfig) { cfg.Maintenance = nil },
		"lifecycle hooks":  func(cfg *InteractionExecutorConfig) { cfg.LifecycleHooks = nil },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := executor.config
			cfg.Lifetime = t.Context()
			cfg.BuildID = interactionTestBuildID
			remove(&cfg)
			if executor, err := NewInteractionExecutor(cfg); err == nil || executor != nil {
				t.Fatalf("incomplete executor = %v, %v", executor, err)
			}
		})
	}
}
