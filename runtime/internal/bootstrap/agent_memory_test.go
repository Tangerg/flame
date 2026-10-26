package bootstrap

import (
	json "encoding/json/v2"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	memorydomain "github.com/Tangerg/flame/plugins/memory/backend/domain"
	memoryrpc "github.com/Tangerg/flame/plugins/memory/backend/rpc"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestMemoryEditSurvivesBackendAndRuntimeRestart(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "memory"))
	source := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "memory"}
	r.poMCPTools(&source, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	item := mustMemoryAdd(r, memoryAddRequest(r, memoryrpc.AddInput{Scope: memorydomain.ScopeProject, CWD: cfg.DefaultWorkspacePath, Content: "original fact"}), "add-memory")
	content := "user correction"
	pinned := true
	request, err := memoryQuery(r, memoryrpc.ListInput{Scope: memorydomain.ScopeProject, CWD: cfg.DefaultWorkspacePath})
	if err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(memoryrpc.UpdateInput{ID: item.ID.String(), Content: &content, Pinned: &pinned})
	if err != nil {
		t.Fatal(err)
	}
	request.Method, request.Input = memoryrpc.Update, string(input)
	reply := r.must(delivery.PluginsCall, request, "edit-memory").(*protocol.CallPluginResponse)
	var edited memorydomain.ItemSnapshot
	if err := json.Unmarshal([]byte(reply.Output), &edited); err != nil {
		t.Fatal(err)
	}
	if edited.ID != item.ID || edited.Content != content || !edited.Pinned || edited.Origin != memorydomain.OriginUser || edited.Status != memorydomain.StatusActive || !edited.CreatedAt.Equal(item.CreatedAt) {
		t.Fatalf("edited memory=%+v", edited)
	}
	r.close()
	r = poMCPOpen(t, cfg)
	r.poMCPTools(&source, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	page := readMemoryPage(r, memoryrpc.ListInput{Scope: memorydomain.ScopeProject, CWD: cfg.DefaultWorkspacePath})

	if len(page.Data) != 1 || !reflect.DeepEqual(page.Data[0], edited) {
		t.Fatalf("restart lost user correction=%+v", page.Data)
	}
	replay := mustMemoryAdd(r, memoryAddRequest(r, memoryrpc.AddInput{Scope: memorydomain.ScopeProject, CWD: cfg.DefaultWorkspacePath, Content: "original fact"}), "add-memory")
	if replay.ID != item.ID {
		t.Fatalf("command replay created another memory=%+v", replay)
	}
}
