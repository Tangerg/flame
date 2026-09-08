package agentexec

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
)

func TestTransientSessionStateRequiresEveryCleanupOwner(t *testing.T) {
	for _, test := range []struct {
		name     string
		contexts *WorkingContextComposer
		tools    *toolset.Resolver
		shells   *exec.Shells
	}{
		{name: "working context", tools: new(toolset.Resolver), shells: new(exec.Shells)},
		{name: "tools", contexts: NewWorkingContextComposer(WorkingContextConfig{}), shells: new(exec.Shells)},
		{name: "shells", contexts: NewWorkingContextComposer(WorkingContextConfig{}), tools: new(toolset.Resolver)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if state, err := NewTransientSessionState(test.contexts, test.tools, test.shells); err == nil || state != nil {
				t.Fatalf("NewTransientSessionState = (%v, %v), want incomplete cleanup rejected", state, err)
			}
		})
	}
}
