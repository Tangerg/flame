package bootstrap

import (
	json "encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
)

func TestUsageReportsRecoversItsOriginalReceiptAfterBackendExitsBeforeReply(t *testing.T) {
	source := filepath.Join(t.TempDir(), "lost reply candidate")
	if err := os.CopyFS(source, os.DirFS(filepath.Join(firstPartyPackages, "usagereports"))); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(source, "backend")); err != nil {
		t.Fatal(err)
	}
	build := exec.CommandContext(t.Context(), "node", "../../../packages/backend-sdk/testing/build.mjs", filepath.Join(source, "backend"), "../../../runtime/internal/testsupport/testdata/usage-reply-loss.ts")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build response-loss backend: %s: %v", output, err)
	}
	authorPluginPackage(t, source)
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, source)
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	contracts, err := plugincontract.ParseCapability([]byte(testsupport.UsageReportsCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.CallPluginRequest{
		Route: protocol.PluginCallRoute{
			InstallationID: installed.ID,
			Digest:         installed.Selected.Digest,
			Contribution:   "usage-reports",
			Contract:       contracts.Identity().String(),
			Method:         testsupport.UsageReportsCaptureV1Method,
			Target:         framecall.Target{Scope: extension.ScopeInstallation, Reference: installed.ID},
		},
		Input: `{"period":"daily"}`,
	}
	recoverLostReply := func(request protocol.CallPluginRequest, key string) *protocol.CallPluginResponse {
		t.Helper()
		options := protocol.CallMetadata{IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)}}
		first := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, options)
		if first.Failure == nil {
			t.Fatalf("lost %s response became a known success", request.Route.Method)
		}
		r = reopenPluginRuntime(t, r, &cfg)
		waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
		recovered := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, options)
		if recovered.Failure != nil {
			t.Fatal(recovered.Failure)
		}
		return recovered.Value.(*protocol.CallPluginResponse)
	}
	reply := recoverLostReply(request, "original-lost-save")
	var saved testsupport.UsageReportsCaptureV1OutputRpcHeader
	if reply.Failure != nil || json.Unmarshal([]byte(reply.Output), &saved) != nil || saved.Id == "" {
		t.Fatalf("original capture was not reconciled: %+v", reply)
	}
	request.Route.Method, request.Input = testsupport.UsageReportsListV2Method, `{}`
	listed := r.must(delivery.PluginsCall, request, "").(*protocol.CallPluginResponse)
	var headers testsupport.UsageReportsListV2Output
	if err := json.Unmarshal([]byte(listed.Output), &headers); err != nil || len(headers.Reports) != 1 || headers.Reports[0].Id != saved.Id || headers.Reports[0].ObservedAt != saved.ObservedAt {
		t.Fatalf("recovery repeated or changed the captured accounting observation: %s: %v", listed.Output, err)
	}
	removal, err := framevalue.EncodeJSON(testsupport.UsageReportsDeleteV2InputRpcReportInput{Id: saved.Id})
	if err != nil {
		t.Fatal(err)
	}
	request.Route.Method, request.Input = testsupport.UsageReportsDeleteV2Method, removal.String()
	removed := recoverLostReply(request, "original-lost-removal")
	var receipt testsupport.UsageReportsDeleteV2Output
	if removed.Failure != nil || json.Unmarshal([]byte(removed.Output), &receipt) != nil || receipt.Id != saved.Id {
		t.Fatalf("original deletion did not retain report %q: %+v", saved.Id, removed)
	}
	request.Route.Method, request.Input = testsupport.UsageReportsListV2Method, `{}`
	listed = r.must(delivery.PluginsCall, request, "").(*protocol.CallPluginResponse)
	if err := json.Unmarshal([]byte(listed.Output), &headers); err != nil || len(headers.Reports) != 0 {
		t.Fatalf("removed report returned after receipt recovery: %s: %v", listed.Output, err)
	}
}

func TestUsageReportsInstalledCaptureKeepsCoreFactsAcrossLaterWorkAndRestart(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "usagereports"))
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	contracts, err := plugincontract.ParseCapability([]byte(testsupport.UsageReportsCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, input any, key string) *protocol.CallPluginResponse {
		t.Helper()
		encoded, err := framevalue.EncodeJSON(input)
		if err != nil {
			t.Fatal(err)
		}
		result := r.must(delivery.PluginsCall, protocol.CallPluginRequest{
			Route: protocol.PluginCallRoute{
				InstallationID: installed.ID,
				Digest:         installed.Selected.Digest,
				Contribution:   "usage-reports",
				Contract:       contracts.Identity().String(),
				Method:         method,
				Target:         framecall.Target{Scope: extension.ScopeInstallation, Reference: installed.ID},
			},
			Input: encoded.String(),
		}, key).(*protocol.CallPluginResponse)
		if result.Failure != nil {
			t.Fatalf("report operation failed: %+v", result.Failure)
		}
		return result
	}
	run := func(key string) {
		t.Helper()
		session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Accounting evidence"}, key+"-session").(*protocol.Session)
		started := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Produce one retained model attempt."}}}, protocol.CallMetadata{IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)}})
		if started.Failure != nil {
			t.Fatal(started.Failure)
		}
		for _, err := range started.Events {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	run("before-capture")
	input := testsupport.UsageReportsCaptureInput{Period: "daily"}
	captured := call(testsupport.UsageReportsCaptureV1Method, input, "save-report")
	var header testsupport.UsageReportsCaptureV1OutputRpcHeader
	if err := json.Unmarshal([]byte(captured.Output), &header); err != nil {
		t.Fatal(err)
	}
	request := testsupport.UsageReportsGetV1InputRpcReportInput{Id: header.Id}
	original := call(testsupport.UsageReportsGetV1Method, request, "").Output
	var saved testsupport.UsageReportsReportGithubComTangergFlameRuntimeProtocolUsageSummary
	if err := json.Unmarshal([]byte(original), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Summary == nil || saved.Summary.Coverage == nil || generatedInteger(t, saved.Summary.Coverage.Calls) == 0 || saved.Since == nil || saved.Summary.Since == nil || saved.ObservedAt != saved.Summary.ObservedAt || *saved.Since != *saved.Summary.Since {
		t.Fatalf("capture lost core evidence or range: %+v", saved)
	}
	run("after-capture")
	r = reopenPluginRuntime(t, r, &cfg)
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	if restored := call(testsupport.UsageReportsGetV1Method, request, "").Output; restored != original {
		t.Fatalf("saved evidence changed after later work and restart: %s", restored)
	}
	if replay := call(testsupport.UsageReportsCaptureV1Method, input, "save-report"); replay.Output != captured.Output {
		t.Fatal("replay did not return the original capture identity")
	}
	call(testsupport.UsageReportsDeleteV2Method, testsupport.UsageReportsDeleteV2InputRpcReportInput{Id: header.Id}, "remove-report")
	call(testsupport.UsageReportsCaptureV1Method, input, "save-report")
	var remaining testsupport.UsageReportsListV2Output
	listed := call(testsupport.UsageReportsListV2Method, struct{}{}, "")
	if err := json.Unmarshal([]byte(listed.Output), &remaining); err != nil || remaining.Reports == nil || len(remaining.Reports) != 0 {
		t.Fatalf("capture replay recreated removed evidence: %s %v", listed.Output, err)
	}
	current := r.must(delivery.UsageSummary, protocol.UsageSummaryRequest{SinceDays: new(1)}, "").(*protocol.UsageSummary)
	if uint64(current.Coverage.Calls) <= generatedInteger(t, saved.Summary.Coverage.Calls) {
		t.Fatal("archive deletion changed or replaced the core accounting ledger")
	}
}
