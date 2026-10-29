package bootstrap

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestDeliverablesInstalledNativeRevisionsAndReceiptsSurviveRuntimeRestart(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	source := filepath.Join(firstPartyPackages, "deliverables")
	installed := poMCPActivate(r, source)
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	encoded, err := os.ReadFile(filepath.Join(source, "io.github.tangerg.flame/contracts/capability.generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	contract, err := portable.ParseCapability(encoded)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, input any, key string) *protocol.CallPluginResponse {
		t.Helper()
		value, err := jsonvalue.Encode(input)
		if err != nil {
			t.Fatal(err)
		}
		return r.must(delivery.PluginsCall, protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "deliverables", Contract: contract.Identity().String(), Method: "flame.deliverables/" + method + ".v1", Input: value.String(), Target: portable.Target{Scope: "installation", Reference: installed.ID}}, key).(*protocol.CallPluginResponse)
	}
	type nativeRevision struct {
		ID         string `json:"id"`
		DocumentID string `json:"documentId"`
		SHA256     string `json:"sha256"`
		Size       int    `json:"size"`
		Parent     string `json:"parent"`
	}
	content := `{"title":"Runtime deliverable","blocks":[{"type":"paragraph","runs":[{"text":"Retained original content"}]}]}`
	input := map[string]any{"title": "Runtime deliverable", "format": "docx", "contentJSON": content, "provenance": map[string]any{"sources": []any{}, "assumptions": []any{}, "units": "", "reproduction": "Isolated public Runtime fixture"}}
	created := call("create", input, "original-deliverable")
	if created.Failure != nil {
		t.Fatal(created.Failure)
	}
	var first nativeRevision
	if err := json.Unmarshal([]byte(created.Output), &first); err != nil || first.ID == "" || first.DocumentID == "" {
		t.Fatalf("invalid native revision: %s: %v", created.Output, err)
	}
	read := call("read", map[string]any{"id": first.DocumentID}, "")
	var opened struct {
		InspectionJSON string `json:"inspectionJSON"`
	}
	if read.Failure != nil || json.Unmarshal([]byte(read.Output), &opened) != nil {
		t.Fatalf("inspection failed: %+v", read)
	}
	var inspection struct {
		Items []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(opened.InspectionJSON), &inspection); err != nil || len(inspection.Items) != 1 {
		t.Fatalf("native inspection was not retained: %s: %v", opened.InspectionJSON, err)
	}
	operations, err := jsonvalue.Encode(map[string]any{"operations": []any{map[string]any{"type": "replaceText", "id": inspection.Items[0].ID, "expect": inspection.Items[0].Text, "text": "Revised native content"}}})
	if err != nil {
		t.Fatal(err)
	}
	editInput := map[string]any{"id": first.DocumentID, "expectedRevision": first.ID, "operationsJSON": operations.String()}
	edited := call("edit", editInput, "original-edit")
	var second nativeRevision
	if edited.Failure != nil || json.Unmarshal([]byte(edited.Output), &second) != nil || second.Parent != first.ID || second.SHA256 == first.SHA256 {
		t.Fatalf("native revision transition failed: %+v", edited)
	}
	stale := call("edit", editInput, "stale-edit")
	if stale.Failure == nil || stale.Failure.Code != "revision_conflict" {
		t.Fatalf("stale revision was not refused: %+v", stale)
	}
	download := call("download", map[string]any{"assetId": second.ID, "offset": 0, "length": 192 * 1024}, "")
	var transfer struct {
		Base64 string `json:"base64"`
		Done   bool   `json:"done"`
	}
	if download.Failure != nil || json.Unmarshal([]byte(download.Output), &transfer) != nil || !transfer.Done {
		t.Fatalf("native transfer failed: %+v", download)
	}
	bytes, err := base64.StdEncoding.DecodeString(transfer.Base64)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(bytes)
	if len(bytes) != second.Size || hex.EncodeToString(hash[:]) != second.SHA256 {
		t.Fatal("download did not preserve the exact native revision")
	}
	r = reopenPluginRuntime(t, r, &cfg)
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	if recovered := call("create", input, "original-deliverable"); recovered.Failure != nil || recovered.Output != created.Output {
		t.Fatalf("original creation receipt changed: %+v", recovered)
	}
	if recovered := call("edit", editInput, "original-edit"); recovered.Failure != nil || recovered.Output != edited.Output {
		t.Fatalf("original edit receipt changed: %+v", recovered)
	}
	current := call("read", map[string]any{"id": first.DocumentID}, "")
	if current.Failure != nil || !strings.Contains(current.Output, "Revised native content") {
		t.Fatalf("native revision did not survive restart: %+v", current)
	}
	old := call("read", map[string]any{"id": first.DocumentID, "revision": first.ID}, "")
	if old.Failure != nil || !strings.Contains(old.Output, "Retained original content") {
		t.Fatalf("historical native bytes were overwritten: %+v", old)
	}
	removed := call("remove", map[string]any{"id": first.DocumentID, "expectedRevision": second.ID}, "remove-deliverable")
	if removed.Failure != nil {
		t.Fatal(removed.Failure)
	}
	call("create", input, "original-deliverable")
	remaining := call("list", map[string]any{"limit": 10}, "")
	if remaining.Failure != nil || remaining.Output != `{"entries":[]}` {
		t.Fatalf("receipt replay recreated removed native content: %+v", remaining)
	}
}
