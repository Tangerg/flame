package bootstrap

import (
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestMemoryEditSurvivesBackendAndRuntimeRestart(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "memory"))
	source := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "memory"}
	r.poMCPTools(&source, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	item := r.must(delivery.AgentMemoryAdd, protocol.AgentMemoryAddRequest{Scope: protocol.AgentMemoryScopeProject, Workspace: &protocol.WorkspaceRef{Path: cfg.DefaultWorkspacePath}, Content: "original fact"}, "add-memory").(*protocol.AgentMemoryItem)
	content := "user correction"
	pinned := true
	edited := r.must(delivery.AgentMemoryUpdate, protocol.AgentMemoryUpdateRequest{ID: item.ID, Content: &content, Pinned: &pinned}, "edit-memory").(*protocol.AgentMemoryItem)
	if edited.ID != item.ID || edited.Content != content || !edited.Pinned || edited.Origin != protocol.AgentMemoryOriginUser || edited.Status != protocol.AgentMemoryStatusActive || !edited.CreatedAt.Equal(item.CreatedAt) {
		t.Fatalf("edited memory=%+v", edited)
	}
	r.close()
	r = poMCPOpen(t, cfg)
	r.poMCPTools(&source, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	page := r.must(delivery.AgentMemoryList, protocol.AgentMemoryListRequest{Scope: protocol.AgentMemoryScopeProject, Workspace: &protocol.WorkspaceRef{Path: cfg.DefaultWorkspacePath}}, "").(*protocol.Page[protocol.AgentMemoryItem])
	if len(page.Data) != 1 || page.Data[0] != *edited {
		t.Fatalf("restart lost user correction=%+v", page.Data)
	}
	replay := r.must(delivery.AgentMemoryAdd, protocol.AgentMemoryAddRequest{Scope: protocol.AgentMemoryScopeProject, Workspace: &protocol.WorkspaceRef{Path: cfg.DefaultWorkspacePath}, Content: "original fact"}, "add-memory").(*protocol.AgentMemoryItem)
	if replay.ID != item.ID {
		t.Fatalf("command replay created another memory=%+v", replay)
	}
}
