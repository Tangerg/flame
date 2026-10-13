package delivery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/testsupport"

	agentmemory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	"github.com/Tangerg/flame/runtime/protocol"
)

func serverAgentMemoryItemID(digit byte) agentmemory.ItemID {
	id, err := agentmemory.ParseItemID(agentmemory.ItemIDPrefix + strings.Repeat(
		string(digit),
		agentmemory.MaximumItemIDCharacters-len(agentmemory.ItemIDPrefix),
	))
	if err != nil {
		panic(err)
	}
	return id
}

// recordingAgentMemory captures application use cases driven by agentMemory.*
// handlers, so Delivery's wire mapping is tested without a persistence port.
type recordingAgentMemory struct {
	addScope   agentmemory.Scope
	addCWD     string
	addContent string

	err error
}

func (r *recordingAgentMemory) Add(_ context.Context, scope agentmemory.Scope, cwd, content string) (agentmemory.Item, error) {
	r.addScope, r.addCWD, r.addContent = scope, cwd, content
	if r.err != nil {
		return agentmemory.Item{}, r.err
	}
	return testsupport.MustAgentMemoryItem(agentmemory.ItemSnapshot{ID: serverAgentMemoryItemID('1'), Scope: scope, Content: content, Origin: agentmemory.OriginUser, Status: agentmemory.StatusActive}), nil
}

func TestAgentMemoryAdd(t *testing.T) {
	rec := &recordingAgentMemory{}
	s := newTestHandler(&stubRuntime{})
	s.agentMemory = rec

	added, err := s.AddAgentMemory(context.Background(), protocol.AgentMemoryAddRequest{
		Scope: "project", Workspace: &protocol.WorkspaceRef{Path: "/repo"}, Content: "- new note",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.addContent != "- new note" || rec.addCWD != "/repo" || added.Origin != "user" {
		t.Fatalf("add recorded=%+v wire=%+v", rec, added)
	}
}

func TestAgentMemoryTargetFullMapsToInvalidParams(t *testing.T) {
	s := newTestHandler(&stubRuntime{})
	s.agentMemory = &recordingAgentMemory{err: agentmemory.ErrTargetFull}
	_, err := s.AddAgentMemory(t.Context(), protocol.AgentMemoryAddRequest{
		Scope: protocol.AgentMemoryScopeUser, Content: "new memory",
	})
	if !errors.Is(err, protocol.ErrInvalidParams) || !errors.Is(err, agentmemory.ErrTargetFull) {
		t.Fatalf("AddAgentMemory error = %v, want invalid_params wrapping ErrTargetFull", err)
	}
}
