package bootstrap

import (
	json "encoding/json/v2"
	"path/filepath"
	"reflect"
	"testing"

	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestMemoryEditSurvivesBackendAndRuntimeRestart(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "memory"))
	source := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "memory"}
	r.poMCPTools(&source, memoryToolsReady)
	item := mustMemoryAdd(r, memoryAddRequest(r, memorycontract.AddV2Input{Scope: "project", CWD: cfg.defaultWorkspacePath, Content: "original fact"}), "add-memory")
	content := "user correction"
	pinned := true
	request, err := memoryQuery(r, memorycontract.ListV2Input{Scope: "project", CWD: cfg.defaultWorkspacePath})
	if err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(memorycontract.UpdateV2Input{ID: string(item.ID), ExpectedRevision: item.Revision, Content: &content, Pinned: &pinned})
	if err != nil {
		t.Fatal(err)
	}
	request.Method, request.Input = memorycontract.UpdateV2Method, string(input)
	reply := r.must(delivery.PluginsCall, request, "edit-memory").(*protocol.CallPluginResponse)
	var edited memorycontract.UpdateV2Output
	if err := json.Unmarshal([]byte(reply.Output), &edited); err != nil {
		t.Fatal(err)
	}
	if edited.ID != item.ID || edited.Content != content || !edited.Pinned || edited.Origin != "user" || edited.Status != "active" || !edited.CreatedAt.Equal(item.CreatedAt) {
		t.Fatalf("edited memory=%+v", edited)
	}
	r.close()
	r = poMCPOpen(t, cfg)
	r.poMCPTools(&source, memoryToolsReady)
	page := readMemoryPage(r, memorycontract.ListV2Input{Scope: "project", CWD: cfg.defaultWorkspacePath})

	if len(page.Data) != 1 || !reflect.DeepEqual(page.Data[0], edited) {
		t.Fatalf("restart lost user correction=%+v", page.Data)
	}
	replay := mustMemoryAdd(r, memoryAddRequest(r, memorycontract.AddV2Input{Scope: "project", CWD: cfg.defaultWorkspacePath, Content: "original fact"}), "add-memory")
	if replay.ID != item.ID {
		t.Fatalf("command replay created another memory=%+v", replay)
	}
}
