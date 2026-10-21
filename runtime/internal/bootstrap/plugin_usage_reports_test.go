package bootstrap

import (
	json "encoding/json/v2"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/plugins/usagereports/backend/rpc"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestUsageReportsInstalledCaptureKeepsCoreFactsAcrossLaterWorkAndRestart(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "usagereports"))
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	contracts, err := rpc.NewContracts[protocol.UsageSummary]()
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
	input := rpc.CaptureInput{Period: "daily"}
	captured := call(contracts.Capture.Name(), input, "save-report")
	var header rpc.Header
	if err := json.Unmarshal([]byte(captured.Output), &header); err != nil {
		t.Fatal(err)
	}
	request := rpc.ReportInput{ID: header.ID}
	original := call(contracts.Get.Name(), request, "").Output
	var saved rpc.Report[protocol.UsageSummary]
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
	var remaining []rpc.Header
	listed := call(contracts.List.Name(), struct{}{}, "")
	if err := json.Unmarshal([]byte(listed.Output), &remaining); err != nil || len(remaining) != 0 {
		t.Fatalf("capture replay recreated removed evidence: %s %v", listed.Output, err)
	}
	current := r.must(delivery.UsageSummary, protocol.UsageSummaryRequest{SinceDays: new(1)}, "").(*protocol.UsageSummary)
	if current.Coverage.Calls <= saved.Summary.Coverage.Calls {
		t.Fatal("archive deletion changed or replaced the core accounting ledger")
	}
}
