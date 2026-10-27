package pluginservices

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestColdToolRestorationUsesTheOriginalGraphWithoutCurrentCatalog(t *testing.T) {
	name := testsupport.ServerName("records")
	release := testsupport.Release(t, "tool-only", plugin.Declaration{Name: "example.tool-only", Servers: []plugin.Server{{Name: name, Transport: mcpserver.TransportStdio, Command: "backend"}}})
	installation, err := plugin.New(testsupport.InstallationID(t), "original source", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	source, err := installation.ServerSource(release, name)
	if err != nil {
		t.Fatal(err)
	}
	remote := sdk.NewServer(&sdk.Implementation{Name: "original-tool-owner"}, nil)
	backend, err := qimumcp.NewBackend(remote)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	var receiptMu sync.Mutex
	var receipt pluginsdk.ToolReceipt
	input := testsupport.JSONInput(t, `{}`)
	query, err := pluginsdk.ToolReceiptQuery()
	if err != nil {
		t.Fatal(err)
	}
	if err := query.RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command qimumcp.Command[qimumcp.ReceiptRequest]) (pluginsdk.ToolReceipt, error) {
		if command.Input.ID != pluginsdk.ToolCommandPrefix+"original-logical-call" || command.Input.Method != "append" || command.Input.InputDigest != input.Digest() {
			return pluginsdk.ToolReceipt{}, errors.New("receipt request changed its original attribution")
		}
		receiptMu.Lock()
		defer receiptMu.Unlock()
		return receipt, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := backend.Publish(); err != nil {
		t.Fatal(err)
	}
	if err := pluginsdk.RegisterTools(remote); err != nil {
		t.Fatal(err)
	}
	remote.AddTool(&sdk.Tool{Name: "append", InputSchema: jsontext.Value(`{"type":"object"}`)}, func(ctx context.Context, _ *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		id, err := pluginsdk.InvocationID(ctx)
		if err != nil || id != "original-logical-call" {
			return nil, errors.Join(errors.New("original logical identity changed"), err)
		}
		calls.Add(1)
		value, err := pluginsdk.NewToolReceipt(chat.NewTextToolOutput("original committed result"), false)
		if err != nil {
			return nil, err
		}
		receiptMu.Lock()
		receipt = value
		receiptMu.Unlock()
		return nil, errors.New("reply unavailable after original commit")
	})
	endpoint := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil))
	t.Cleanup(endpoint.Close)
	callbacks := qimumcp.NewCallbacks()
	t.Cleanup(func() { _ = callbacks.Close() })
	store := &toolInstallationStore{snapshot: plugin.Snapshot{Installation: installation, Selected: release}}
	managed := newManagedFixture(t, store, mcpserver.Server{Source: source, Name: name, Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: endpoint.URL}, callbacks, nil)
	var current []toolcontract.Tool
	managed.Pool.SetToolSink(func(tools []toolcontract.Tool, _ []mcpserver.ID) { current = tools })
	if len(current) != 1 {
		t.Fatalf("original catalog = %d tools", len(current))
	}
	ref, err := toolset.Identify(current[0])
	if err != nil {
		t.Fatal(err)
	}
	authority, err := toolset.SourceFingerprint(current[0], ref)
	if err != nil {
		t.Fatal(err)
	}
	activation, err := toolset.InstallationActivation(current[0])
	if err != nil {
		t.Fatal(err)
	}
	bound, err := toolcontract.Bind(current[0])
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := bound.Contract().Prepare(chat.ToolCall{ID: "physical-call", Name: bound.Contract().Definition().Name, Arguments: input.String()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := operation.Adopt(t.Context(), "original-logical-call", extension.Model, "run_original")
	if _, err := bound.Call(ctx, invocation); err == nil || calls.Load() != 1 {
		t.Fatalf("original mutation did not reach its owned receipt: calls=%d, %v", calls.Load(), err)
	}
	checkpoint := runs.ExecutorCheckpoint{RootMemberID: "member_original", SessionID: "ses_original", BuildID: testsupport.BuildID, Payload: []byte("original continuation"), Bindings: []plugin.Activation{activation}}
	if _, err := managed.Stores.ExecContext(t.Context(), `INSERT INTO runs(run_id,session_id,state,created_at,updated_at) VALUES('run_original','ses_original','waiting',0,0)`); err != nil {
		t.Fatal(err)
	}
	if err := managed.Stores.CommitPluginReferences(t.Context(), managed.Host, "original-checkpoint", checkpoint, func(ctx context.Context) error {
		return managed.Stores.ExecutorCheckpoints.SaveCheckpoint(ctx, checkpoint)
	}); err != nil {
		t.Fatal(err)
	}
	managed.close(t)
	managed.open(t, false)
	checkpoint, err = managed.Stores.ExecutorCheckpoints.LoadCheckpoint(t.Context(), checkpoint.RootMemberID)
	if err != nil || len(checkpoint.Bindings) != 1 || !checkpoint.Bindings[0].Equal(activation) {
		t.Fatalf("cold checkpoint lost the original graph: %+v, %v", checkpoint.Bindings, err)
	}
	results, err := NewToolResults(store, managed.Pool)
	if err != nil {
		t.Fatal(err)
	}
	output, err := results.OriginalToolResult(t.Context(), ref, release.Digest(), authority, checkpoint.Bindings[0], "original-logical-call", input.String())
	if err != nil || output == nil || calls.Load() != 1 {
		t.Fatalf("cold result restoration replayed or changed ownership: %+v, calls=%d, %v", output, calls.Load(), err)
	}
	if text, valid := output.Text(); !valid || text != "original committed result" {
		t.Fatalf("original receipt text changed: %q, %t", text, valid)
	}
	built, err := toolset.Build(t.Context(), toolset.BuildConfig{Lifetime: t.Context(), UserHome: t.TempDir(), RestoreMCP: managed.Pool.RestoreTools})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, close := range built.Closers {
			if err := close(); err != nil {
				t.Error(err)
			}
		}
	})
	workspace := t.TempDir()
	ctx = executionctx.WithScope(t.Context(), runs.ExecutionScope{SessionID: "ses_original", CWD: workspace, WorkspaceCWD: workspace})
	manifest, err := built.Resolver.Manifest(ctx, domaintool.GroupRoot, &toolset.Retained{MCP: []domaintool.Ref{ref}, Bindings: checkpoint.Bindings, Resources: []plugin.Dependency{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := manifest.Close(); err != nil {
			t.Error(err)
		}
	}()
	if len(manifest.Bindings) != 1 || !manifest.Bindings[0].Equal(activation) || calls.Load() != 1 {
		t.Fatalf("manifest did not restore exact original authority: %+v, calls=%d", manifest.Bindings, calls.Load())
	}
	fresh, err := built.Resolver.Manifest(ctx, domaintool.GroupRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := fresh.Close(); err != nil {
			t.Error(err)
		}
	}()
	for _, executable := range append(fresh.Visible, fresh.Deferred...) {
		actual, err := toolset.Identify(executable)
		if err != nil {
			t.Fatal(err)
		}
		if actual == ref {
			t.Fatal("restoring original work published its graph as today's catalog")
		}
	}
}
