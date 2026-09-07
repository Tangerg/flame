package agentexec

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
)

func TestTransientSessionStateRequiresItsCleanupOwners(t *testing.T) {
	for name, construct := range map[string]func() (*TransientSessionState, error){
		"working context": func() (*TransientSessionState, error) {
			return NewTransientSessionState(nil, &toolset.Resolver{}, &exec.Shells{})
		},
		"tools": func() (*TransientSessionState, error) {
			return NewTransientSessionState(&WorkingContextComposer{}, nil, &exec.Shells{})
		},
		"shells": func() (*TransientSessionState, error) {
			return NewTransientSessionState(&WorkingContextComposer{}, &toolset.Resolver{}, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if state, err := construct(); err == nil || state != nil {
				t.Fatalf("incomplete cleanup construction = %v, %v", state, err)
			}
		})
	}
}
