package bootstrap

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestReviewExecutableCLIIdentityConflictPreservesOriginalReceipt(t *testing.T) {
	runtime := poMCPOpen(t, poMCPConfig(t))
	defer runtime.close()
	installed := poMCPActivate(runtime, reviewExamplePackage(t))
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "reviews"}
	runtime.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	contribution := installed.Selected.Contributions[0]
	request := protocol.CallPluginRequest{
		InstallationID: installed.ID, Digest: installed.Selected.Digest,
		Contribution: contribution.ID, Contract: contribution.Contract,
		Target: capability.Target{Scope: "installation", Reference: installed.ID},
		Method: "io.github.tangerg.flame.examples/reviews.list", Input: `{}`,
	}
	read := func() jsoninput.Input {
		t.Helper()
		query := request
		query.Method, query.Input = "io.github.tangerg.flame.examples/reviews.list", `{}`
		reply := runtime.must(delivery.PluginsCall, query, "").(*protocol.CallPluginResponse)
		value, err := jsoninput.Parse([]byte(reply.Output))
		if err != nil || reply.Failure != nil {
			t.Fatalf("read private review: %+v, %v", reply, err)
		}
		return value
	}
	var initial struct {
		Reviews []map[string]jsontext.Value `json:"reviews"`
	}
	if err := json.Unmarshal(read().JSON(), &initial, json.RejectUnknownMembers(true)); err != nil || len(initial.Reviews) != 1 {
		t.Fatalf("read the owned review: %+v, %v", initial, err)
	}
	encode := func(value any) jsoninput.Input {
		t.Helper()
		encoded, err := jsoninput.Encode(value)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	resolved := maps.Clone(initial.Reviews[0])
	resolved["revision"], resolved["status"] = jsontext.Value(`2`), jsontext.Value(`"resolved"`)
	opened := maps.Clone(initial.Reviews[0])
	opened["revision"], opened["status"] = jsontext.Value(`3`), jsontext.Value(`"open"`)
	original := encode(map[string]any{"type": "updated", "review": resolved})
	advanced := encode(map[string]any{"type": "updated", "review": opened})
	invoke, closeCLI := reviewCLIInvocation(t, runtime)
	defer closeCLI()
	request.Method = "io.github.tangerg.flame.examples/reviews.update"
	call := func(key, input string) ([]byte, []byte, error) {
		t.Helper()
		request.Input = input
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		return invoke("plugins", "call", "--request", string(encoded), "--command-id", key)
	}
	assertReply := func(output []byte, want jsoninput.Input) {
		t.Helper()
		var reply protocol.CallPluginResponse
		if err := json.Unmarshal(output, &reply, json.RejectUnknownMembers(true)); err != nil {
			t.Fatalf("CLI lost the structured business result: %s, %v", output, err)
		}
		actual, err := jsoninput.Parse([]byte(reply.Output))
		if err != nil || reply.Failure != nil || actual != want {
			t.Fatalf("original outcome was replaced: %+v, want %s, %v", reply, want.JSON(), err)
		}
	}
	success := func(key, input string, want jsoninput.Input) {
		t.Helper()
		output, stderr, err := call(key, input)
		if err != nil {
			t.Fatalf("CLI rejected an original binding: stdout=%s stderr=%s error=%v", output, stderr, err)
		}
		assertReply(output, want)
	}
	const (
		key   = "cli_ffffffffffffffffffffffffffffffff"
		first = `{"id":"example-review","expectedRevision":1,"status":"resolved"}`
		next  = `{"id":"example-review","expectedRevision":2,"status":"open"}`
	)
	success(key, first, original)
	for range 2 {
		output, stderr, err := call(key, next)
		if err == nil || len(output) != 0 || !strings.Contains(string(stderr), protocol.ErrIdempotencyConflict.Error()) || strings.Contains(string(stderr), protocol.ErrInternalError.Error()) {
			t.Fatalf("changed input reached business execution or uncertainty: stdout=%s stderr=%s error=%v", output, stderr, err)
		}
	}
	refused := runtime.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, delivery.Options{IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if refused.Value != nil || refused.Failure == nil || !errors.Is(refused.Failure, protocol.ErrIdempotencyConflict) || refused.Failure.Problem().Type != protocol.ErrIdempotencyConflict.Error() {
		t.Fatalf("Go and CLI entrances disagree about the original binding: %+v", refused)
	}
	if current, want := read(), encode(map[string]any{"reviews": []any{resolved}}); current != want {
		t.Fatalf("identity conflict changed the private review: %s, want %s", current.JSON(), want.JSON())
	}
	success("cli_00000000000000000000000000000000", next, advanced)
	for range 2 {
		success(key, first, original)
	}
	request.Input = first
	recovered := runtime.must(delivery.PluginsCall, request, key).(*protocol.CallPluginResponse)
	assertReply(encode(recovered).JSON(), original)
	if current, want := read(), encode(map[string]any{"reviews": []any{opened}}); current != want {
		t.Fatalf("original receipt recovery changed current private state: %s, want %s", current.JSON(), want.JSON())
	}
}
