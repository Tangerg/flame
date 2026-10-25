package plugin_test

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	plugin "github.com/Tangerg/flame/packages/plugin-sdk"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type typescriptInput struct {
	Mode  string         `json:"mode"`
	Value jsontext.Value `json:"value"`
}
type typescriptOutput struct {
	Value  jsontext.Value `json:"value"`
	Source string         `json:"source"`
}
type typescriptHostInput struct {
	Value jsontext.Value `json:"value"`
}

type typescriptDiagnostics struct {
	sync.Mutex
	text strings.Builder
}

func (d *typescriptDiagnostics) Write(value []byte) (int, error) {
	d.Lock()
	defer d.Unlock()
	return d.text.Write(value)
}
func (d *typescriptDiagnostics) String() string { d.Lock(); defer d.Unlock(); return d.text.String() }

func TestTypeScriptBackendImplementsPublishedGoContract(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate conformance source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	distribution := t.TempDir()
	prepare := exec.CommandContext(t.Context(), "node", filepath.Join(root, "tooling/node-companion.mjs"), "--os", runtime.GOOS, "--arch", runtime.GOARCH, "--output", distribution, "--offline")
	if output, err := prepare.CombinedOutput(); err != nil {
		t.Fatalf("prepare companion: %s: %v", output, err)
	}
	node, err := localruntime.NodeForExecutable(filepath.Join(distribution, "flame"))
	if err != nil {
		t.Fatal(err)
	}
	if err := node.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	packageDirectory := filepath.Join(distribution, "package with 空格")
	build := exec.CommandContext(t.Context(), "node", filepath.Join(root, "packages/plugin-sdk/typescript/testing/build.mjs"), packageDirectory)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build backend: %s: %v", output, err)
	}
	read, err := plugin.NewMethod[typescriptInput, typescriptOutput](plugin.MethodConfig{Name: "fixture.read", Kind: capability.Query, Target: "installation", Description: "Read exact original input."})
	if err != nil {
		t.Fatal(err)
	}
	write, err := plugin.NewMethod[typescriptInput, typescriptOutput](plugin.MethodConfig{Name: "fixture.write", Kind: capability.Mutation, Target: "installation", Description: "Commit one original outcome."})
	if err != nil {
		t.Fatal(err)
	}
	queryStream, err := plugin.NewStreamingMethod[typescriptInput, jsontext.Value, typescriptOutput](plugin.MethodConfig{Name: "fixture.query-stream", Kind: capability.Query, Target: "installation", Description: "Read exact chunks and final."})
	if err != nil {
		t.Fatal(err)
	}
	commandStream, err := plugin.NewStreamingMethod[typescriptInput, jsontext.Value, typescriptOutput](plugin.MethodConfig{Name: "fixture.command-stream", Kind: capability.Mutation, Target: "installation", Description: "Commit streamed final."})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := capability.New(capability.Config{Reference: "flame.test.typescript", Composition: capability.Collection, Scope: "installation"}, read.Descriptor(), write.Descriptor(), queryStream.Descriptor(), commandStream.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	hostMethod, err := plugin.NewMethod[typescriptHostInput, typescriptHostInput](plugin.MethodConfig{Name: "fixture.host", Kind: capability.Query, Target: "installation", Description: "Read through the original calling owner."})
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{"contract.json": contract.Descriptor(), "host.json": hostMethod.Descriptor()} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(packageDirectory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command(node.Command(), node.Arguments(filepath.Join(packageDirectory, "start.mjs"))...)
	command.Env = []string{"PATH="}
	var diagnostics typescriptDiagnostics
	command.Stderr = &diagnostics
	client := sdk.NewClient(&sdk.Implementation{Name: "go.conformance"}, nil)
	if err := plugin.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(t.Context(), &sdk.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect: %v; stderr: %s", err, diagnostics.String())
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := plugin.SetUp(t.Context(), session, []plugin.SetupContribution{{ID: "fixture", Export: "fixture", Requires: []plugin.SetupDependency{}}}, sdk.ParamsBase{}); err != nil {
		t.Fatalf("setup: %v; stderr: %s", err, diagnostics.String())
	}
	if err := plugin.Activate(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	bound, err := plugin.ResolveCapability(t.Context(), session, "fixture", contract)
	if err != nil {
		t.Fatalf("resolve: %v; stderr: %s", err, diagnostics.String())
	}
	readBinding, err := read.Bind(bound)
	if err != nil {
		t.Fatal(err)
	}
	input := typescriptInput{Mode: "echo", Value: jsontext.Value(`{"wide":9007199254740993,"decimal":1.0,"exponent":1e+25}`)}
	exact, err := jsoninput.Parse(input.Value)
	if err != nil {
		t.Fatal(err)
	}
	input.Value = exact.JSON()
	output, err := readBinding.Call(t.Context(), input)
	if err != nil || string(output.Value) != string(input.Value) {
		t.Fatalf("exact round trip: %s, %v", output.Value, err)
	}
	original, err := jsoninput.Encode(input)
	if err != nil || output.Source != original.String() {
		t.Fatalf("original evidence changed: %s, %v", output.Source, err)
	}
	writeBinding, err := write.Bind(bound)
	if err != nil {
		t.Fatal(err)
	}
	input.Mode = "invalid"
	if _, err := writeBinding.CallCommand(t.Context(), "original", input); err == nil {
		t.Fatal("committed invalid output became success")
	}
	recovered, found, err := writeBinding.ReadReceipt(t.Context(), "original", input)
	if err != nil || !found || string(recovered.Value) != string(input.Value) {
		t.Fatalf("original receipt: %s, %v, %v", recovered.Value, found, err)
	}
	host := plugin.NewCallbacks()
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	host.AdmitSession(session)
	invocation := capability.Invocation{ID: "host-call", Origin: capability.Model, Parent: "parent", Target: capability.Target{Scope: "installation", Reference: "original-target"}}
	ctx, err := plugin.WithInvocation(t.Context(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := plugin.NewCallback(hostMethod, nil, func(calling context.Context, command plugin.Command[typescriptHostInput]) (typescriptHostInput, error) {
		actual, err := plugin.InvocationFromContext(calling)
		if err != nil || actual != invocation {
			t.Errorf("callback lost original invocation: %+v, %v", actual, err)
		}
		return command.Input, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := host.Open(ctx, session, plugin.CallbackLimits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20}, receiver)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	metadata, err := plugin.CallbackMetadata(ref)
	if err != nil {
		t.Fatal(err)
	}
	input.Mode = "host"
	output, err = readBinding.CallWithMetadata(ctx, input, metadata)
	if err != nil || string(output.Value) != string(input.Value) {
		t.Fatalf("host callback: %s, %v", output.Value, err)
	}
	if err := metadata.Set(plugin.InvocationMetadataKey, invocation); err != nil {
		t.Fatal(err)
	}
	arguments, err := jsoninput.Encode(typescriptHostInput{Value: input.Value})
	if err != nil {
		t.Fatal(err)
	}
	toolResult, err := session.CallTool(ctx, &sdk.CallToolParams{Meta: metadata.Meta, Name: "host_echo", Arguments: arguments.JSON()})
	if err != nil || toolResult.IsError {
		t.Fatalf("ordinary Tool callback: %+v: %v", toolResult, err)
	}
	toolReceipt, err := plugin.ReadToolReceipt(ctx, session, invocation.ID, "host_echo", arguments)
	if err != nil || toolReceipt == nil {
		t.Fatalf("original Tool receipt: %+v: %v", toolReceipt, err)
	}
	if text, found := toolReceipt.Text(); !found || text != arguments.String() {
		t.Fatalf("Tool receipt changed original exact evidence: %+v", toolReceipt)
	}
	for _, message := range []string{"conformance module loaded", "default console dependency loaded", "named console dependency loaded"} {
		if !strings.Contains(diagnostics.String(), message) {
			t.Fatalf("module initialization logging did not reach stderr: %s", message)
		}
	}
	queryBinding, err := bound.Method(queryStream.Descriptor().Name)
	if err != nil {
		t.Fatal(err)
	}
	limits := plugin.CallbackLimits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20}
	for _, mode := range []string{"echo", "mutated-final", "missing", "late"} {
		input.Mode = mode
		original, err := jsoninput.Encode(input)
		if err != nil {
			t.Fatal(err)
		}
		chunks, finals, failures := 0, 0, 0
		for frame, err := range queryStream.StreamWithMetadata(t.Context(), queryBinding, host, limits, "", input, sdk.ParamsBase{}) {
			if err != nil {
				failures++
				continue
			}
			if frame.Chunk != nil {
				chunks++
				if string(*frame.Chunk) != string(input.Value) {
					t.Fatalf("stream rounded exact chunk: %s", *frame.Chunk)
				}
			}
			if frame.Result != nil {
				finals++
				if frame.Result.Source != original.String() {
					t.Fatalf("stream %s changed its admitted final result: %+v", mode, frame.Result)
				}
			}
		}
		complete := mode == "echo" || mode == "mutated-final"
		if chunks != 1 || complete && (finals != 1 || failures != 0) || !complete && (finals != 0 || failures != 1) {
			t.Fatalf("stream %s: chunks=%d finals=%d failures=%d", mode, chunks, finals, failures)
		}
	}
	commandBinding, err := bound.Method(commandStream.Descriptor().Name)
	if err != nil {
		t.Fatal(err)
	}
	input.Mode = "late"
	for _, err := range commandStream.StreamWithMetadata(t.Context(), commandBinding, host, limits, "stream-original", input, sdk.ParamsBase{}) {
		if err == nil {
			continue
		}
	}
	originalStream, err := jsoninput.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	settled, found, err := commandBinding.ReadReceipt(t.Context(), "stream-original", originalStream)
	if err != nil || !found || !strings.Contains(settled.String(), "9007199254740993") {
		t.Fatalf("interrupted command stream lost its original final receipt: %s, %v, %v", settled, found, err)
	}
	input.Mode = "wait"
	for frame, err := range queryStream.StreamWithMetadata(t.Context(), queryBinding, host, limits, "", input, sdk.ParamsBase{}) {
		if err != nil {
			t.Fatal(err)
		}
		if frame.Chunk != nil {
			break
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(packageDirectory, "cancelled.txt")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("withdrawn stream did not cancel and close its source")
		}
		time.Sleep(time.Millisecond)
	}
}
