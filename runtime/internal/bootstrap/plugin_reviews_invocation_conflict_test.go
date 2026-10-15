package bootstrap

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestReviewPrivateInvocationConflictMatchesModelToolAndExecutableCLI(t *testing.T) {
	url, closeBackend := reviewRuntimeBackend(t, "TestInvocationConflictRuntimeFixture")
	defer closeBackend()
	source := reviewExamplePackage(t)
	descriptor, err := json.Marshal(map[string]any{
		"$schema":    "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
		"mcpServers": map[string]any{"reviews": map[string]any{"type": "streamable-http", "url": url}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "mcp.json"), descriptor, 0600); err != nil {
		t.Fatal(err)
	}
	const input = `{"id":"example-review","expectedRevision":1,"status":"resolved"}`
	cfg := poMCPConfig(t)
	cfg.ApprovalMode = approval.ModeYolo
	cfg.ChatResolver = testChatResolver(reviewRefusalChat(t, input))
	runtime := poMCPOpen(t, cfg)
	defer runtime.close()
	installed := poMCPActivate(runtime, source)
	server := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "reviews"}
	runtime.poMCPTools(&server, func(tools []protocol.MCPTool) bool { return len(tools) == 2 })
	contribution := installed.Selected.Contributions[0]
	request := protocol.CallPluginRequest{
		InstallationID: installed.ID, Digest: installed.Selected.Digest,
		Contribution: contribution.ID, Contract: contribution.Contract,
		Target: capability.Target{Scope: "installation", Reference: installed.ID},
		Method: "io.github.tangerg.flame.examples/reviews.update", Input: input,
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
	before := read()
	const refusal = `{"type":"invocationConflict"}`
	assertReviewToolRefusal(t, runtime, refusal)
	invoke, closeCLI := reviewCLIInvocation(t, runtime)
	defer closeCLI()
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	arguments := []string{"plugins", "call", "--request", string(encoded), "--command-id", "cli_11111111111111111111111111111111"}
	assertReply := func(output []byte, expected jsoninput.Input) {
		t.Helper()
		var reply protocol.CallPluginResponse
		if err := json.Unmarshal(output, &reply, json.RejectUnknownMembers(true)); err != nil {
			t.Fatalf("CLI lost the structured business result: %s, %v", output, err)
		}
		actual, err := jsoninput.Parse([]byte(reply.Output))
		if err != nil || reply.Failure != nil || actual != expected {
			t.Fatalf("CLI changed the business outcome: %+v, want %s, %v", reply, expected.JSON(), err)
		}
	}
	want, err := jsoninput.Parse([]byte(refusal))
	if err != nil {
		t.Fatal(err)
	}
	output, stderr, err := invoke(arguments...)
	if err != nil {
		t.Fatalf("CLI lost the private identity refusal: stdout=%s stderr=%s error=%v", output, stderr, err)
	}
	assertReply(output, want)
	if after := read(); after != before {
		t.Fatalf("conflicting Tool or CLI mutated the review: %s, want %s", after.JSON(), before.JSON())
	}
	// The existing private identity owns different input. Recovery must refuse
	// that receipt instead of overwriting it or redispatching the new request.
	output, stderr, err = invoke(arguments...)
	if err == nil || len(output) != 0 || !strings.Contains(string(stderr), protocol.ErrInternalError.Error()) || strings.Contains(string(stderr), protocol.ErrIdempotencyConflict.Error()) {
		t.Fatalf("conflicting private receipt became a definite recovery: stdout=%s stderr=%s error=%v", output, stderr, err)
	}
	recovered := runtime.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, delivery.Options{
		IdempotencyKey: arguments[len(arguments)-1], RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion},
	})
	if recovered.Value != nil || recovered.Failure == nil || !errors.Is(recovered.Failure, protocol.ErrInternalError) || recovered.Failure.Problem().Type != protocol.ErrInternalError.Error() {
		t.Fatalf("Go accepted another input's private receipt: %+v", recovered)
	}
	if after := read(); after != before {
		t.Fatalf("conflicting receipt recovery mutated the review: %s, want %s", after.JSON(), before.JSON())
	}
	var initial struct {
		Reviews []map[string]jsontext.Value `json:"reviews"`
	}
	if err := json.Unmarshal(before.JSON(), &initial, json.RejectUnknownMembers(true)); err != nil || len(initial.Reviews) != 1 {
		t.Fatalf("decode original review: %+v, %v", initial, err)
	}
	resolved := maps.Clone(initial.Reviews[0])
	resolved["revision"], resolved["status"] = jsontext.Value(`2`), jsontext.Value(`"resolved"`)
	updated, err := jsoninput.Encode(map[string]any{"type": "updated", "review": resolved})
	if err != nil {
		t.Fatal(err)
	}
	arguments[len(arguments)-1] = "cli_22222222222222222222222222222222"
	output, stderr, err = invoke(arguments...)
	if err != nil {
		t.Fatalf("private conflict prevented a fresh command: stdout=%s stderr=%s error=%v", output, stderr, err)
	}
	assertReply(output, updated)
	expected, err := jsoninput.Encode(map[string]any{"reviews": []any{resolved}})
	if err != nil {
		t.Fatal(err)
	}
	if current := read(); current != expected {
		t.Fatalf("fresh command did not advance the original owner once: %s, want %s", current.JSON(), expected.JSON())
	}
}
