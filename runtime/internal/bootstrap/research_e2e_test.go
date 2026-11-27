package bootstrap

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
)

func TestResearchKeepsPageEvidenceAndHidesRevokedSources(t *testing.T) {
	provider := httptest.NewServer(&knowledgeProvider{t: t})
	t.Cleanup(provider.Close)
	cfg := poMCPConfig(t)
	cfg.modelProviders = nil
	cfg.defaultSelection = testsupport.MustModelSelection("openai-compatible", "knowledge-model")
	r := poMCPOpen(t, cfg)
	activateModelProviders(r)
	if _, err := r.host.application.delivery.Models.ListProviders(t.Context()); err != nil {
		t.Fatal(err)
	}
	baseURL, token := provider.URL+"/v1", "local-research-fixture"
	r.must(delivery.ProvidersUpdate, protocol.UpdateProviderRequest{Provider: "openai",
		BaseURL: &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &baseURL},
		APIKey:  &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &token},
	}, "research-embedding-endpoint")
	r.must(delivery.ModelsSetEmbeddingRole, protocol.EmbeddingRole{Provider: "openai", Model: "text-embedding-3-small"}, "research-embedding")
	deliverables := r.installAndInspect(filepath.Join(firstPartyPackages, "deliverables"), "install-research-deliverables")
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: deliverables.ID, Enabled: true}, "enable-research-deliverables")
	waitCallContributorState(t, r, deliverables.ID, protocol.MCPServerConnected)
	deliverables = r.pluginInstallation(deliverables.ID)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "knowledgebase"))
	contract, err := plugincontract.ParseCapability([]byte(testsupport.KnowledgebaseCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, input any, key string) *protocol.CallPluginResponse {
		t.Helper()
		encoded, err := framevalue.EncodeJSON(input)
		if err != nil {
			t.Fatal(err)
		}
		return r.must(delivery.PluginsCall, protocol.CallPluginRequest{
			Route: protocol.PluginCallRoute{
				InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "knowledgebase-operations",
				Contract: contract.Identity().String(), Method: method,
				Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: installed.ID},
			},
			Input: encoded.String(),
		}, key).(*protocol.CallPluginResponse)
	}
	nativeCall := func(method string, input any, key string, out any) {
		t.Helper()
		contribution := slices.IndexFunc(deliverables.Selected.Contributions, func(value protocol.PluginContribution) bool {
			return value.ID == "deliverables"
		})
		if contribution < 0 {
			t.Fatal("deliverables capability is missing")
		}
		contract, err := plugincontract.ParseCapability([]byte(deliverables.Selected.Contributions[contribution].Contract))
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := framevalue.EncodeJSON(input)
		if err != nil {
			t.Fatal(err)
		}
		response := r.must(delivery.PluginsCall, protocol.CallPluginRequest{
			Route: protocol.PluginCallRoute{InstallationID: deliverables.ID, Digest: deliverables.Selected.Digest,
				Contribution: "deliverables", Contract: contract.Identity().String(), Method: "flame.deliverables/" + method + ".v1",
				Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: deliverables.ID}},
			Input: encoded.String(),
		}, key).(*protocol.CallPluginResponse)
		if response.Failure != nil {
			t.Fatal(response.Failure)
		}
		if err := json.Unmarshal([]byte(response.Output), out); err != nil {
			t.Fatalf("%s output: %v", method, err)
		}
	}
	first := r.must(delivery.Name("projects.create"), protocol.CreateProjectRequest{Name: "Payments research"}, "research-new").(*protocol.Project)
	second := r.must(delivery.Name("projects.create"), protocol.CreateProjectRequest{Name: "Independent work"}, "research-other").(*protocol.Project)
	context := func(title, key string) *protocol.Session {
		return r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: title, ProjectID: first.ID}, key).(*protocol.Session)
	}
	started := context("Research", "research-session")
	importSource := func(id, project, text string, expected int, key string) {
		t.Helper()
		result := call(testsupport.KnowledgebaseImportV1Method, map[string]any{
			"content":          map[string]any{"id": id, "title": "Payment guide", "source": id + ".txt", "project": project, "text": text},
			"expectedRevision": expected,
		}, key)
		if result.Failure != nil {
			t.Fatal(result.Failure)
		}
	}
	const old = "Overview\fLimit 30\nIndependent metric 12\f"
	importSource("limit", first.ID, old, 0, "research-import-limit")
	importSource("check", first.ID, "Independent checks\fLimit verified\nMetric verified\f", 0, "research-import-check")
	importSource("other", second.ID, "Other Project private text", 0, "research-import-other")
	accept := func(id, statement string, line int) {
		t.Helper()
		result := call(testsupport.KnowledgebaseAcceptClaimV1Method, map[string]any{
			"project": first.ID, "id": id, "expectedRevision": 0, "statement": statement,
			"reason": "Both passages inspected",
			"citations": []map[string]any{
				{"documentId": "limit", "revision": 1, "page": 2, "firstLine": line, "lastLine": line},
				{"documentId": "check", "revision": 1, "page": 2, "firstLine": line, "lastLine": line},
			},
		}, "research-accept-"+id)
		if result.Failure != nil {
			t.Fatal(result.Failure)
		}
	}
	accept("limit", "Limit is 30", 1)
	accept("metric", "Independent metric is 12", 2)
	const originalContent = `{"title":"Payments","slides":[{"title":"Limit evidence","elements":[{"type":"chart","chart":"bar","title":"Verified limit","x":1,"y":1,"width":8,"height":4,"series":[{"name":"Limit","labels":["Payments"],"values":[30]}]}]}]}`
	var chart struct {
		ID         string `json:"id"`
		DocumentID string `json:"documentId"`
	}
	nativeCall("create", map[string]any{
		"format": "pptx", "title": "Verified limits", "project": first.ID, "contentJSON": originalContent,
		"provenance": map[string]any{"sources": []string{}, "assumptions": []string{}, "units": "count", "reproduction": "Compare the two inspected source lines."},
	}, "create-research-chart", &chart)
	documentID, revision := chart.DocumentID, chart.ID
	if documentID == "" || revision == "" {
		t.Fatalf("native chart create did not identify its revision: %+v", chart)
	}
	var projected struct {
		LayoutJSON string `json:"layoutJSON"`
	}
	nativeCall("slides", map[string]string{"id": documentID, "revision": revision}, "", &projected)
	var layout struct {
		Slides []struct {
			Elements []struct {
				ID    string `json:"id"`
				Kind  string `json:"kind"`
				Chart *struct {
					Data jsontext.Value `json:"data"`
				} `json:"chart"`
			} `json:"elements"`
		} `json:"slides"`
	}
	if err := json.Unmarshal([]byte(projected.LayoutJSON), &layout); err != nil {
		t.Fatal(err)
	}
	if len(layout.Slides) != 1 {
		t.Fatalf("invalid chart layout: %s", projected.LayoutJSON)
	}
	chartIndex := slices.IndexFunc(layout.Slides[0].Elements, func(element struct {
		ID    string `json:"id"`
		Kind  string `json:"kind"`
		Chart *struct {
			Data jsontext.Value `json:"data"`
		} `json:"chart"`
	}) bool {
		return element.Kind == "chart"
	})
	if chartIndex < 0 || layout.Slides[0].Elements[chartIndex].Chart == nil {
		t.Fatalf("invalid native chart: %s", projected.LayoutJSON)
	}
	element := layout.Slides[0].Elements[chartIndex]
	method := map[string]string{"name": "Two-source verification", "instructions": "Compare the original page lines, then update the native chart."}
	acceptWork := func(id string) testsupport.KnowledgebaseAcceptWorkV1OutputWireWorkReceipt {
		t.Helper()
		result := call(testsupport.KnowledgebaseAcceptWorkV1Method, map[string]any{
			"project": first.ID, "id": id, "expectedRevision": 0, "title": "Limit is 30",
			"method": method, "claims": []map[string]any{{"id": "limit", "revision": 1}},
			"artifact": map[string]string{"installationId": deliverables.ID, "id": documentID, "revision": revision, "object": element.ID},
		}, "accept-work-"+id)
		if result.Failure != nil {
			t.Fatal(result.Failure)
		}
		var receipt testsupport.KnowledgebaseAcceptWorkV1OutputWireWorkReceipt
		if err := json.Unmarshal([]byte(result.Output), &receipt); err != nil {
			t.Fatal(err)
		}
		return receipt
	}
	work := acceptWork("limit-work")
	if original := acceptWork("limit-work"); original.MethodVersion != work.MethodVersion || original.Revision.String() != work.Revision.String() {
		t.Fatal("replay replaced the accepted work")
	}
	if second := acceptWork("same-method"); second.MethodVersion != work.MethodVersion || second.MethodVersion == "" {
		t.Fatal("reusing the same procedure invented a new method version")
	}
	readWork := func() testsupport.KnowledgebaseWorkView {
		t.Helper()
		result := call(testsupport.KnowledgebaseReadWorkV1Method, map[string]any{"project": first.ID, "id": work.Id}, "")
		if result.Failure != nil {
			t.Fatal(result.Failure)
		}
		var view testsupport.KnowledgebaseWorkView
		if err := json.Unmarshal([]byte(result.Output), &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	if current := readWork(); current.State != "current" || current.Evidence == nil || len(current.Claims) != 1 || len(current.Claims[0].Evidence) != 2 {
		t.Fatalf("accepted work lacks exact page lines and native revision: %+v", current)
	}
	operations, err := json.Marshal(map[string]any{"operations": []any{map[string]any{
		"type": "setChartData", "id": element.ID, "expect": element.Chart.Data,
		"data": map[string]any{"categories": []string{"Payments"}, "series": []any{map[string]any{"name": "Limit", "values": []int{20}}}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	nativeCall("edit", map[string]string{"id": documentID, "expectedRevision": revision, "operationsJSON": string(operations)}, "update-research-chart", &chart)
	updatedRevision := chart.ID
	if updatedRevision == "" || updatedRevision == revision {
		t.Fatalf("editing the chart did not create a new native revision: %+v", chart)
	}
	importSource("limit", first.ID, "Overview\fLimit 20\nIndependent metric 12\f", 1, "research-update-limit")

	r = reopenPluginRuntime(t, r, &cfg)
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	if changed := readWork(); changed.State != "recheck" || changed.Artifact == nil || changed.Evidence == nil ||
		changed.Artifact.Revision != revision || changed.Evidence.Head == nil || *changed.Evidence.Head != updatedRevision ||
		!changed.Evidence.ObjectAtHead || changed.Claims[0].State != "recheck" {
		t.Fatalf("restart lost the chart head or source invalidation: %+v", changed)
	}
	var exported struct {
		ID string `json:"id"`
	}
	nativeCall("export", map[string]string{"id": documentID, "revision": updatedRevision, "kind": "native"}, "export-current-chart", &exported)
	var chunk struct {
		Base64 string `json:"base64"`
		Done   bool   `json:"done"`
	}
	nativeCall("download", map[string]any{"assetId": exported.ID, "offset": 0, "length": 196608}, "", &chunk)
	native, err := base64.StdEncoding.DecodeString(chunk.Base64)
	if err != nil || !chunk.Done {
		t.Fatalf("native chart export incomplete: %v", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(native), int64(len(native)))
	if err != nil {
		t.Fatal(err)
	}
	editable := false
	for _, part := range archive.File {
		if !strings.HasPrefix(part.Name, "ppt/charts/chart") || !strings.HasSuffix(part.Name, ".xml") {
			continue
		}
		reader, err := part.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		editable = editable || bytes.Contains(raw, []byte(">20<"))
	}
	if !editable {
		t.Fatal("the editable PPTX did not contain the latest chart data")
	}
	reopened := context("Review evidence", "research-reopen")
	if reopened.ID == started.ID || reopened.ProjectID != started.ProjectID {
		t.Fatal("the independent Session did not retain the Project identity")
	}
	read := func(id string) testsupport.KnowledgebaseReadClaimV1OutputWireClaimRead {
		t.Helper()
		result := call(testsupport.KnowledgebaseReadClaimV1Method, map[string]any{
			"project": reopened.ProjectID, "id": id, "revision": 1,
		}, "")
		var claim testsupport.KnowledgebaseReadClaimV1OutputWireClaimRead
		if result.Failure != nil {
			t.Fatal(result.Failure)
		}
		if err := json.Unmarshal([]byte(result.Output), &claim); err != nil {
			t.Fatal(err)
		}
		return claim
	}
	limit, metric := read("limit"), read("metric")
	if limit.State != "recheck" || len(limit.Evidence) != 2 ||
		limit.Evidence[0].Excerpt != "Limit 30" || !limit.Evidence[0].NeedsRecheck ||
		limit.Evidence[1].NeedsRecheck || metric.State != "current" {
		t.Fatalf("source update did not preserve original evidence and localize recheck: limit=%+v metric=%+v", limit, metric)
	}

	inaccessible := call(testsupport.KnowledgebaseReadV1Method, map[string]any{"project": first.ID, "id": "other", "page": 1}, "")
	if inaccessible.Failure == nil || inaccessible.Failure.Code != "not_found" {
		t.Fatalf("same-title material escaped the Project boundary: %+v", inaccessible)
	}
	moved := call(testsupport.KnowledgebaseMoveV1Method, map[string]any{
		"id": "limit", "project": first.ID, "targetProject": second.ID, "expectedRevision": 2,
	}, "research-revoke")
	if moved.Failure != nil {
		t.Fatal(moved.Failure)
	}
	listed := call(testsupport.KnowledgebaseListClaimsV1Method, map[string]any{"project": first.ID}, "")
	if listed.Failure != nil || strings.Contains(listed.Output, "Limit is 30") || strings.Contains(listed.Output, "Independent metric is 12") {
		t.Fatalf("claim list exposed stale source text: %+v", listed)
	}
	revoked := read("limit")
	if revoked.State != "unavailable" || revoked.Statement != nil || revoked.Reason != nil || len(revoked.Evidence) != 0 {
		t.Fatalf("revoked evidence leaked by historical link: %+v", revoked)
	}
	if hidden := readWork(); hidden.State != "recheck" || hidden.Title != "" || hidden.Artifact != nil ||
		hidden.Evidence != nil || hidden.Method != nil || len(hidden.Claims) != 1 || hidden.Claims[0].Statement != nil {
		t.Fatalf("research work retained links or content derived from revoked material: %+v", hidden)
	}
	oldLink := call(testsupport.KnowledgebaseReadV1Method, map[string]any{
		"project": first.ID, "id": "limit", "revision": 1, "page": 2,
	}, "")
	if oldLink.Failure == nil || oldLink.Failure.Code != "not_found" {
		t.Fatalf("revoked historical page remained readable: %+v", oldLink)
	}
}
