package bootstrap

import (
	"testing"
	"time"

	workspaceadapter "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestMemoryContentEditSurvivesCurationAndRuntimeRestart(t *testing.T) {
	cfg := poMCPConfig(t)
	project, err := (workspaceadapter.Resolver{}).ResolveExistingDir(cfg.DefaultWorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, time.July, 19, 4, 0, 0, 0, time.UTC)
	facts, err := cfg.Stores.AgentMemory.AppendLedger(t.Context(), agentmemory.FactBatch{
		Project: project, SessionID: "ses_memory", Day: createdAt.Format(time.DateOnly),
		Facts: []string{"automatic proposal"}, CapturedAt: createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	publication, err := agentmemory.NewPublication(project, agentmemory.State{}, facts[0].Sequence, []string{"automatic proposal"}, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	if published, err := cfg.Stores.AgentMemory.Reconcile(t.Context(), publication); err != nil || !published {
		t.Fatalf("first curation = %t, %v", published, err)
	}
	r := poMCPOpen(t, cfg)
	query := protocol.AgentMemoryListRequest{Scope: protocol.AgentMemoryScopeProject, Workspace: &protocol.WorkspaceRef{Path: project}}
	page := r.must(delivery.AgentMemoryList, query, "").(*protocol.Page[protocol.AgentMemoryItem])
	if len(page.Data) != 1 || page.Data[0].Status != protocol.AgentMemoryStatusPending {
		t.Fatalf("proposal = %+v", page.Data)
	}
	content := "user correction"
	edited := r.must(delivery.AgentMemoryUpdate, protocol.AgentMemoryUpdateRequest{ID: page.Data[0].ID, Content: &content}, "edit-memory").(*protocol.AgentMemoryItem)
	if edited.Origin != protocol.AgentMemoryOriginUser || edited.Status != protocol.AgentMemoryStatusActive ||
		edited.Content != content || edited.ID != page.Data[0].ID || !edited.CreatedAt.Equal(page.Data[0].CreatedAt) {
		t.Fatalf("edited memory = %+v", edited)
	}
	facts, err = cfg.Stores.AgentMemory.AppendLedger(t.Context(), agentmemory.FactBatch{
		Project: project, SessionID: "ses_memory", Day: createdAt.Format(time.DateOnly),
		Facts: []string{"later fact"}, CapturedAt: createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	next, err := agentmemory.NewPublication(project, publication.State(), facts[0].Sequence, nil, edited.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if published, err := cfg.Stores.AgentMemory.Reconcile(t.Context(), next); err != nil || !published {
		t.Fatalf("next curation = %t, %v", published, err)
	}
	r.close()
	r = poMCPOpen(t, cfg)
	page = r.must(delivery.AgentMemoryList, query, "").(*protocol.Page[protocol.AgentMemoryItem])
	if len(page.Data) != 1 || page.Data[0] != *edited {
		t.Fatalf("restart lost the user correction: %+v", page.Data)
	}
}
