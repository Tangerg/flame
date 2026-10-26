package bootstrap

import (
	json "encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	reports "github.com/Tangerg/flame/plugins/usagereports"
	"github.com/Tangerg/flame/runtime/internal/delivery"
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
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, source)
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	contracts, err := reports.NewContracts[protocol.UsageSummary]()
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "usage-reports", Contract: contracts.Capability.Identity().String(), Method: contracts.Capture.Name(), Input: `{"period":"daily"}`, Target: capability.Target{Scope: "installation", Reference: installed.ID}}
	options := delivery.Options{IdempotencyKey: "original-lost-save", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}}
	first := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, options)
	if first.Failure == nil {
		t.Fatal("lost backend response became a known success")
	}
	r.close()
	r = poMCPOpen(t, cfg)
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	recovered := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, options)
	if recovered.Failure != nil {
		t.Fatal(recovered.Failure)
	}
	reply := recovered.Value.(*protocol.CallPluginResponse)
	var saved reports.Header
	if reply.Failure != nil || json.Unmarshal([]byte(reply.Output), &saved) != nil || saved.ID == "" {
		t.Fatalf("original capture was not reconciled: %+v", reply)
	}
	request.Method, request.Input = contracts.List.Name(), `{}`
	listed := r.must(delivery.PluginsCall, request, "").(*protocol.CallPluginResponse)
	var headers []reports.Header
	if err := json.Unmarshal([]byte(listed.Output), &headers); err != nil || len(headers) != 1 || headers[0].ID != saved.ID || !headers[0].ObservedAt.Equal(saved.ObservedAt) {
		t.Fatalf("recovery repeated or changed the captured accounting observation: %s: %v", listed.Output, err)
	}
}

func TestUsageReportsInstalledCaptureKeepsCoreFactsAcrossLaterWorkAndRestart(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "usagereports"))
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	contracts, err := reports.NewContracts[protocol.UsageSummary]()
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, input any, key string) *protocol.CallPluginResponse {
		t.Helper()
		encoded, err := jsoninput.Encode(input)
		if err != nil {
			t.Fatal(err)
		}
		result := r.must(delivery.PluginsCall, protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "usage-reports", Contract: contracts.Capability.Identity().String(), Method: method, Input: encoded.String(), Target: capability.Target{Scope: "installation", Reference: installed.ID}}, key).(*protocol.CallPluginResponse)
		if result.Failure != nil {
			t.Fatalf("report operation failed: %+v", result.Failure)
		}
		return result
	}
	run := func(key string) {
		t.Helper()
		session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Accounting evidence"}, key+"-session").(*protocol.Session)
		started := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Produce one retained model attempt."}}}, delivery.Options{IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
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
	input := reports.CaptureInput{Period: "daily"}
	captured := call(contracts.Capture.Name(), input, "save-report")
	var header reports.Header
	if err := json.Unmarshal([]byte(captured.Output), &header); err != nil {
		t.Fatal(err)
	}
	request := reports.ReportInput{ID: header.ID}
	original := call(contracts.Get.Name(), request, "").Output
	var saved reports.Report[protocol.UsageSummary]
	if err := json.Unmarshal([]byte(original), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Summary.Coverage.Calls == 0 || saved.Since == nil || saved.Summary.Since == nil || !saved.ObservedAt.Equal(saved.Summary.ObservedAt) || !saved.Since.Equal(*saved.Summary.Since) {
		t.Fatalf("capture lost core evidence or range: %+v", saved)
	}
	run("after-capture")
	r.close()
	r = poMCPOpen(t, cfg)
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	if restored := call(contracts.Get.Name(), request, "").Output; restored != original {
		t.Fatalf("saved evidence changed after later work and restart: %s", restored)
	}
	if replay := call(contracts.Capture.Name(), input, "save-report"); replay.Output != captured.Output {
		t.Fatal("replay did not return the original capture identity")
	}
	call(contracts.Delete.Name(), request, "remove-report")
	call(contracts.Capture.Name(), input, "save-report")
	var remaining []reports.Header
	listed := call(contracts.List.Name(), struct{}{}, "")
	if err := json.Unmarshal([]byte(listed.Output), &remaining); err != nil || len(remaining) != 0 {
		t.Fatalf("capture replay recreated removed evidence: %s %v", listed.Output, err)
	}
	current := r.must(delivery.UsageSummary, protocol.UsageSummaryRequest{SinceDays: new(1)}, "").(*protocol.UsageSummary)
	if current.Coverage.Calls <= saved.Summary.Coverage.Calls {
		t.Fatal("archive deletion changed or replaced the core accounting ledger")
	}
}
