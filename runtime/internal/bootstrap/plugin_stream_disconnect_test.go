package bootstrap

import (
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestInstalledStreamProcessExitRetainsOutputAndNeverRedispatchesOriginalCommand(t *testing.T) {
	binary := buildCallContributor(t)
	output, err := exec.CommandContext(t.Context(), binary, "describe-stream-drop", "").CombinedOutput()
	if err != nil {
		t.Fatalf("describe public stream contract: %s, %v", output, err)
	}
	contract, err := portable.ParseCapability(output)
	if err != nil {
		t.Fatal(err)
	}
	var starts, receipts atomic.Int32
	trace := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var event struct {
			Phase string `json:"phase"`
		}
		if err := json.UnmarshalRead(request.Body, &event); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch event.Phase {
		case "started":
			starts.Add(1)
		case "receipt":
			receipts.Add(1)
		default:
			t.Errorf("unknown stream evidence: %s", event.Phase)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer trace.Close()
	r := poMCPOpen(t, poMCPConfig(t))
	defer r.close()
	installed := installCallContributor(t, r, binary, "stream-drop", trace.URL, contract)
	request := protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "contributor", Contract: contract.Identity().String(), Method: contract.Descriptor().Methods[0].Name, Input: `{}`, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: installed.ID}}
	invoke := func() delivery.Result {
		return r.endpoint.Invoke(t.Context(), delivery.PluginsStream, request, delivery.Options{IdempotencyKey: "original-stream", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	}
	first := invoke()
	if first.Failure != nil {
		t.Fatalf("stream did not start: %v", first.Failure)
	}
	chunks, finals, failures := 0, 0, 0
	for raw, err := range first.Events {
		if err != nil {
			failures++
			continue
		}
		event := raw.(protocol.PluginStreamEvent)
		switch event.Kind {
		case protocol.PluginStreamChunk:
			chunks++
			if event.Value != `"witnessed output"` {
				t.Fatalf("disconnect changed observed output: %+v", event)
			}
		case protocol.PluginStreamResult:
			finals++
		case protocol.PluginStreamFailure:
			t.Fatalf("process exit became a definite business refusal: %+v", event)
		}
	}
	if chunks != 1 || finals != 0 || failures != 1 || starts.Load() != 1 {
		t.Fatalf("process exit replayed or completed output: chunks=%d finals=%d failures=%d starts=%d", chunks, finals, failures, starts.Load())
	}
	refused := r.endpoint.Invoke(t.Context(), delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: installed.ID}, delivery.Options{IdempotencyKey: "uninstall-unknown", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if !errors.Is(refused.Failure, protocol.ErrPluginInUse) {
		t.Fatalf("unknown original command released its source dependency: %v", refused.Failure)
	}
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "contributor"}
	r.must(delivery.MCPServersReconnect, protocol.MCPServerRequest{Server: server}, "reconnect-original-source")
	waitCallContributor(t, r, installed.ID)
	retry := invoke()
	unknown := retry.Failure != nil
	if retry.Events != nil {
		for value, err := range retry.Events {
			if err != nil {
				unknown = true
				continue
			}
			t.Fatalf("missing original receipt replayed or synthesized stream evidence: %+v", value)
		}
	}
	if !unknown || starts.Load() != 1 || receipts.Load() == 0 {
		t.Fatalf("original unknown stream was resubmitted or settled: unknown=%v starts=%d receipts=%d", unknown, starts.Load(), receipts.Load())
	}
}
