package bootstrap

import (
	json "encoding/json/v2"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestReviewTypedInputRefusalPreservesIdentityAcrossExecutableCLI(t *testing.T) {
	runtime := poMCPOpen(t, poMCPConfig(t))
	defer runtime.close()
	installed := poMCPActivate(runtime, reviewExamplePackage(t))
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "reviews"}
	runtime.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	contribution := installed.Selected.Contributions[0]
	request := protocol.CallPluginRequest{
		InstallationID: installed.ID, Digest: installed.Selected.Digest,
		Contribution: contribution.ID, Contract: contribution.Contract,
		Target: portable.Target{Scope: extension.ScopeInstallation, Reference: installed.ID},
		Method: "io.github.tangerg.flame.examples/reviews.list", Input: `{}`,
	}
	before := runtime.must(delivery.PluginsCall, request, "").(*protocol.CallPluginResponse).Output
	invoke, closeCLI := reviewCLIInvocation(t, runtime)
	defer closeCLI()
	const key = "cli_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	call := func() ([]byte, []byte, error) {
		t.Helper()
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		return invoke("plugins", "call", "--request", string(encoded), "--command-id", key)
	}
	request.Method = "io.github.tangerg.flame.examples/reviews.update"
	for _, number := range []string{"0.0", "1e0"} {
		request.Input = `{"id":"example-review","expectedRevision":` + number + `,"status":"resolved"}`
		for range 2 {
			output, stderr, err := call()
			if err == nil || len(output) != 0 || !strings.Contains(string(stderr), protocol.ErrInvalidParams.Error()) {
				t.Fatalf("typed structural refusal became execution uncertainty: input=%s stdout=%s stderr=%s error=%v", request.Input, output, stderr, err)
			}
		}
		reply := runtime.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, delivery.Options{IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
		if reply.Value != nil || !errors.Is(reply.Failure, protocol.ErrInvalidParams) {
			t.Fatalf("Go entrance lost the typed structural refusal: %+v", reply)
		}
	}
	request.Input = `{"id":"example-review","expectedRevision":0,"status":"resolved"}`
	want, err := jsonvalue.Parse([]byte(`{"type":"invalidUpdate"}`))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		output, stderr, err := call()
		if err != nil {
			t.Fatalf("structural refusal consumed the original identity: stdout=%s stderr=%s error=%v", output, stderr, err)
		}
		var reply protocol.CallPluginResponse
		if err := json.Unmarshal(output, &reply, json.RejectUnknownMembers(true)); err != nil {
			t.Fatal(err)
		}
		actual, err := jsonvalue.Parse([]byte(reply.Output))
		if err != nil || actual != want || reply.Failure != nil {
			t.Fatalf("admitted business refusal lost its original receipt: %+v, %v", reply, err)
		}
	}
	request.Input = `{"id":"example-review","expectedRevision":1,"status":"resolved"}`
	output, stderr, err := call()
	if err == nil || len(output) != 0 || !strings.Contains(string(stderr), protocol.ErrIdempotencyConflict.Error()) {
		t.Fatalf("admitted refusal released its identity for another mutation: stdout=%s stderr=%s error=%v", output, stderr, err)
	}
	request.Method, request.Input = "io.github.tangerg.flame.examples/reviews.list", `{}`
	after := runtime.must(delivery.PluginsCall, request, "").(*protocol.CallPluginResponse).Output
	if before != after {
		t.Fatalf("input admission changed private review state: before=%s after=%s", before, after)
	}
}
