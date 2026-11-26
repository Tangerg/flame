package bootstrap

import (
	json "encoding/json/v2"
	"net/http/httptest"
	"path/filepath"
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
			InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "knowledgebase-operations",
			Contract: contract.Identity().String(), Method: method, Input: encoded.String(),
			Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: installed.ID},
		}, key).(*protocol.CallPluginResponse)
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
	importSource("limit", first.ID, "Overview\fLimit 20\nIndependent metric 12\f", 1, "research-update-limit")

	reopened := context("Review evidence", "research-reopen")
	if reopened.ID == started.ID || reopened.ProjectID != started.ProjectID {
		t.Fatal("the independent Session did not retain the Project identity")
	}
	read := func(id string) testsupport.KnowledgebaseClaimRead {
		t.Helper()
		result := call(testsupport.KnowledgebaseReadClaimV1Method, map[string]any{
			"project": reopened.ProjectID, "id": id, "revision": 1,
		}, "")
		var claim testsupport.KnowledgebaseClaimRead
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
	oldLink := call(testsupport.KnowledgebaseReadV1Method, map[string]any{
		"project": first.ID, "id": "limit", "revision": 1, "page": 2,
	}, "")
	if oldLink.Failure == nil || oldLink.Failure.Code != "not_found" {
		t.Fatalf("revoked historical page remained readable: %+v", oldLink)
	}
}
