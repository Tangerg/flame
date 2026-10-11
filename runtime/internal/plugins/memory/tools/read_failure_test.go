package tools

import (
	"context"
	"errors"
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	agentmemory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	scopemcp "github.com/Tangerg/scope/mcp"
)

func projectScoped(t *testing.T) context.Context {
	t.Helper()
	return scopemcp.WithRequestMeta(t.Context(), map[string]any{managedmcp.ToolScopeKey: managedmcp.ToolScope{CWD: t.TempDir()}})
}

type failingMemorySearch struct{ err error }

func (f failingMemorySearch) Search(
	context.Context, string, string, int,
) ([]agentmemory.Item, error) {
	return nil, f.err
}

func TestReadToolServiceFailureIsDefinite(t *testing.T) {
	executable, err := NewAgentMemorySearch(failingMemorySearch{
		err: errors.New("memory index unavailable"),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, callErr := callTextTool(projectScoped(t), executable, `{"query":"deploy"}`)
	failure, classified := errors.AsType[*toolcontract.Failure](callErr)
	if !classified {
		t.Fatalf("search failure = %v, want a classified Tool failure", callErr)
	}
	if failure.Kind() != toolcontract.FailureKindFailed || failure.Validate() != nil {
		t.Fatalf("failure kind = %q, valid = %v", failure.Kind(), failure.Validate())
	}
	if !errors.Is(callErr, context.Canceled) && failure.Cause() == nil {
		t.Fatal("classified failure lost its cause")
	}
}

func TestReadToolCancellationStaysUnclassified(t *testing.T) {
	executable, err := NewAgentMemorySearch(failingMemorySearch{err: context.Canceled})
	if err != nil {
		t.Fatal(err)
	}
	_, callErr := callTextTool(projectScoped(t), executable, `{"query":"deploy"}`)
	if !errors.Is(callErr, context.Canceled) {
		t.Fatalf("cancellation = %v, want context.Canceled", callErr)
	}
	if _, classified := errors.AsType[*toolcontract.Failure](callErr); classified {
		t.Fatal("cancellation was turned into model feedback")
	}
}
