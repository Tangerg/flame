package bootstrap

import (
	json "encoding/json/v2"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	memorydomain "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	memoryrpc "github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
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
	request, err := memoryQuery(r, memoryrpc.ListInput{Scope: memorydomain.ScopeProject, CWD: cfg.DefaultWorkspacePath})
	if err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(memoryrpc.UpdateInput{ID: item.ID, Content: &content, Pinned: &pinned})
	if err != nil {
		t.Fatal(err)
	}
	request.Method, request.Input = memoryrpc.Update, input
	reply := r.must(delivery.PluginsCall, request, "edit-memory").(*protocol.CallPluginResponse)
	var edited memorydomain.ItemSnapshot
	if err := json.Unmarshal(reply.Output, &edited); err != nil {
		t.Fatal(err)
	}
	if edited.ID.String() != item.ID || edited.Content != content || !edited.Pinned || edited.Origin != memorydomain.OriginUser || edited.Status != memorydomain.StatusActive || !edited.CreatedAt.Equal(item.CreatedAt) {
		t.Fatalf("edited memory=%+v", edited)
	}
	r.close()
	r = poMCPOpen(t, cfg)
	r.poMCPTools(&source, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	page := readMemoryPage(r, memoryrpc.ListInput{Scope: memorydomain.ScopeProject, CWD: cfg.DefaultWorkspacePath})
	expected := protocol.AgentMemoryItem{ID: edited.ID.String(), Scope: protocol.AgentMemoryScope(edited.Scope), Content: edited.Content, Origin: protocol.AgentMemoryOrigin(edited.Origin), Status: protocol.AgentMemoryStatus(edited.Status), Pinned: edited.Pinned, SessionID: edited.SessionID, Day: edited.Day, CreatedAt: edited.CreatedAt, UpdatedAt: edited.UpdatedAt}
	if len(page.Data) != 1 || page.Data[0] != expected {
		t.Fatalf("restart lost user correction=%+v", page.Data)
	}
	replay := r.must(delivery.AgentMemoryAdd, protocol.AgentMemoryAddRequest{Scope: protocol.AgentMemoryScopeProject, Workspace: &protocol.WorkspaceRef{Path: cfg.DefaultWorkspacePath}, Content: "original fact"}, "add-memory").(*protocol.AgentMemoryItem)
	if replay.ID != item.ID {
		t.Fatalf("command replay created another memory=%+v", replay)
	}
}
