package agentexec

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
)

type failingMemoryPartition struct {
	scope agentmemory.Scope
}

func (f failingMemoryPartition) Items(_ context.Context, scope agentmemory.Scope, _ string) ([]agentmemory.Item, error) {
	if scope == f.scope {
		return nil, errors.New("memory storage unavailable")
	}
	return []agentmemory.Item{{Content: "healthy partition fact", Pinned: true}}, nil
}

func TestPinnedMemoryFailurePreservesHealthyPartitionAndDiagnostics(t *testing.T) {
	for _, scope := range []agentmemory.Scope{agentmemory.ScopeProject, agentmemory.ScopeUser} {
		t.Run(string(scope), func(t *testing.T) {
			var diagnostics bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&diagnostics, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })
			composer := newTestWorkingContextComposer(t, WorkingContextConfig{AgentMemory: failingMemoryPartition{scope: scope}})
			message, err := composer.composeSystemMessage(t.Context(), t.TempDir())
			if err != nil || !strings.Contains(message.Text(), "healthy partition fact") {
				t.Fatalf("healthy memory lost: message=%q error=%v", message.Text(), err)
			}
			if output := diagnostics.String(); !strings.Contains(output, "memory storage unavailable") || !strings.Contains(output, "scope="+string(scope)) {
				t.Fatalf("missing scoped memory failure diagnostic: %q", output)
			}
		})
	}
}

func TestRecallFailureRemainsNonBlockingAndDiagnostic(t *testing.T) {
	var diagnostics bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&diagnostics, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	composer := newTestWorkingContextComposer(t, WorkingContextConfig{
		AgentMemorySearch: &fakeAgentMemorySearcher{err: errors.New("memory search unavailable")},
	})
	_, found, err := composer.recallMessage(t.Context(), "/repo", "private query")
	if err != nil || found {
		t.Fatalf("failed recall found=%t error=%v", found, err)
	}
	if output := diagnostics.String(); !strings.Contains(output, "memory search unavailable") || strings.Contains(output, "private query") {
		t.Fatalf("invalid recall failure diagnostic: %q", output)
	}
}
