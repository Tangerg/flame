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
	pinnedID string
	pinned   bool
	editedID string
	editedTx string

	addScope   agentmemory.Scope
	addCWD     string
	addContent string

	getItem agentmemory.Item
	err     error
}

func (r *recordingAgentMemory) Update(_ context.Context, id string, content *string, pinned *bool) (agentmemory.Item, error) {
	if content != nil {
		r.editedID, r.editedTx = id, *content
	}
	if pinned != nil {
		r.pinnedID, r.pinned = id, *pinned
	}
	if r.err != nil {
		return agentmemory.Item{}, r.err
	}
	return r.getItem, nil
}

func (r *recordingAgentMemory) Add(_ context.Context, scope agentmemory.Scope, cwd, content string) (agentmemory.Item, error) {
	r.addScope, r.addCWD, r.addContent = scope, cwd, content
	if r.err != nil {
		return agentmemory.Item{}, r.err
	}
	return testsupport.MustAgentMemoryItem(agentmemory.ItemSnapshot{ID: serverAgentMemoryItemID('1'), Scope: scope, Content: content, Origin: agentmemory.OriginUser, Status: agentmemory.StatusActive}), nil
}

func TestAgentMemoryUpdateAndAdd(t *testing.T) {
	itemID := serverAgentMemoryItemID('a')
	rec := &recordingAgentMemory{getItem: testsupport.MustAgentMemoryItem(agentmemory.ItemSnapshot{
		ID: itemID, Scope: agentmemory.ScopeProject, Content: "- edited", Origin: agentmemory.OriginUser,
		Pinned: true, Status: agentmemory.StatusActive,
	})}
	s := newTestHandler(&stubRuntime{})
	s.agentMemory = rec

	content := "- edited"
	pinned := true
	out, err := s.UpdateAgentMemory(context.Background(), protocol.AgentMemoryUpdateRequest{ID: itemID.String(), Content: &content, Pinned: &pinned})
	if err != nil {
		t.Fatal(err)
	}
	if rec.editedID != itemID.String() || rec.editedTx != "- edited" || rec.pinnedID != itemID.String() || !rec.pinned {
		t.Fatalf("update recorded %+v", rec)
	}
	if out.ID != itemID.String() || !out.Pinned {
		t.Fatalf("update wire = %+v", out)
	}

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

func TestHiddenAgentMemoryMapsToInvalidParams(t *testing.T) {
	s := newTestHandler(&stubRuntime{})
	s.agentMemory = &recordingAgentMemory{err: agentmemory.ErrNotVisible}
	pinned := true
	_, err := s.UpdateAgentMemory(t.Context(), protocol.AgentMemoryUpdateRequest{
		ID: serverAgentMemoryItemID('1').String(), Pinned: &pinned,
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("UpdateAgentMemory error = %v, want invalid_params", err)
	}
}
