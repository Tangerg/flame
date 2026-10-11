package tools

import (
	"context"
	"testing"

	agentmemory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
)

type agentMemorySearchStub struct{}

func (agentMemorySearchStub) Search(context.Context, string, string, int) ([]agentmemory.Item, error) {
	return nil, nil
}

func TestSearchRejectsMissingPort(t *testing.T) {
	for _, search := range []AgentMemorySearch{nil, (*agentMemorySearchStub)(nil)} {
		executable, err := NewAgentMemorySearch(search)
		if err == nil || executable != nil {
			t.Fatalf("missing search port = (%v, %v)", executable, err)
		}
	}
}

func TestNewUsesSearchMemoryContract(t *testing.T) {
	tl, err := NewAgentMemorySearch(agentMemorySearchStub{})
	if err != nil {
		t.Fatal(err)
	}
	if got := tl.Definition().Name; got != "search_memory" {
		t.Fatalf("name = %q, want search_memory", got)
	}
	if _, err := callTextTool(t.Context(), tl, `{"query":"naming","limit":21}`); err == nil {
		t.Fatal("expected an error for limit above 20")
	}
	if _, err := callTextTool(t.Context(), tl, `{"query":"naming","limit":0}`); err == nil {
		t.Fatal("expected an error for numeric default sentinel")
	}
	if _, err := callTextTool(t.Context(), tl, `{"query":"naming","unknown":true}`); err == nil {
		t.Fatal("expected an error for an unknown argument")
	}
}
