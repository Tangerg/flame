package main

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type markdownClient struct {
	method    *pluginmcp.StreamingMethod[plugin.DocumentIdentity, string, plugin.DocumentIdentity]
	binding   *pluginmcp.Binding
	callbacks *pluginmcp.Callbacks
	session   *sdk.ClientSession
}

func openMarkdownClient(t *testing.T) markdownClient {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "trajectory-renderer"}, nil)
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	method, err := plugin.RenderSessionMarkdownMethod()
	if err != nil {
		t.Fatal(err)
	}
	if err := method.RegisterServer(backend, renderMarkdown); err != nil {
		t.Fatal(err)
	}
	contract, err := plugin.SessionMarkdownContract()
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.BindCapability("markdown", contract); err != nil {
		t.Fatal(err)
	}
	if err := backend.Publish(); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "trajectory-host"}, nil)
	if err := pluginmcp.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	left, right := sdk.NewInMemoryTransports()
	connection, err := server.Connect(t.Context(), left, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	session, err := client.Connect(t.Context(), right, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	callbacks := pluginmcp.NewCallbacks()
	t.Cleanup(func() { _ = callbacks.Close() })
	if err := callbacks.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	exported, err := pluginmcp.ResolveCapability(t.Context(), session, "markdown", contract)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := exported.Method(method.Descriptor().Name)
	if err != nil {
		t.Fatal(err)
	}
	return markdownClient{method: method, binding: binding, callbacks: callbacks, session: session}
}

func (c markdownClient) metadata(t *testing.T, read func(plugin.DocumentRange) (string, error)) sdk.ParamsBase {
	t.Helper()
	method, err := plugin.ReadDocumentRangeMethod()
	if err != nil {
		t.Fatal(err)
	}
	callback, err := pluginmcp.NewCallback(method, nil, func(_ context.Context, cmd pluginmcp.Command[plugin.DocumentRange]) (string, error) {
		return read(cmd.Input)
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := c.callbacks.Open(t.Context(), c.session, pluginmcp.CallbackLimits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20}, callback)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	metadata, err := pluginmcp.CallbackMetadata(ref)
	if err != nil {
		t.Fatal(err)
	}
	return metadata
}

func TestMarkdownStreamRendersOriginalToolEvidenceAndVerifiesCompleteDocument(t *testing.T) {
	client := openMarkdownClient(t)
	fullText := "first original line\n````\n# inert heading\n" + strings.Repeat("界🙂 retained evidence\n", 7000) + "last original line"
	artifact := protocol.SessionArtifact{
		Version: protocol.SessionArtifactVersion, Session: protocol.ArtifactSession{ID: "session-original", Title: "Captured session"},
		Items: []protocol.ArtifactItem{
			{ID: "text-original", RunID: "run-original", Type: protocol.ItemTypeToolCall, Status: protocol.ItemStatusCompleted,
				Tool: &protocol.ToolInvocation{Name: "read-original", Arguments: map[string]any{"revision": jsontext.Value(`9007199254740993`)}, Result: fullText}},
			{ID: "structured-original", RunID: "run-original", Type: protocol.ItemTypeToolCall, Status: protocol.ItemStatusIncomplete,
				Error: &protocol.ArtifactProblem{Type: protocol.ArtifactProblemToolFailed, Detail: "recorded execution failure"},
				Tool:  &protocol.ToolInvocation{Name: "inspect-original", Arguments: map[string]any{}, Result: jsontext.Value(`{"revision":9007199254740993,"decimal":1.2300,"exponent":1e+100,"zero":-0,"values":[null,9007199254740995]}`)}},
			{ID: "null-original", RunID: "run-original", Type: protocol.ItemTypeToolCall, Status: protocol.ItemStatusCompleted,
				Tool: &protocol.ToolInvocation{Name: "null-result", Arguments: map[string]any{}, Result: jsontext.Value(`null`)}},
			{ID: "absent-original", RunID: "run-original", Type: protocol.ItemTypeToolCall, Status: protocol.ItemStatusIncomplete,
				Tool: &protocol.ToolInvocation{Name: "absent-result", Arguments: map[string]any{}}},
		},
		ToolResults: []protocol.ArtifactToolResult{{ID: "model-text-original", ItemID: "structured-original", ModelOnly: true, Body: "unique diagnostic not present in result"}},
	}
	source, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	identity := plugin.IdentifyDocument(source)
	var reads atomic.Int64
	metadata := client.metadata(t, func(request plugin.DocumentRange) (string, error) {
		if request.Offset != (reads.Add(1)-1)*plugin.SessionDocumentChunkBytes || request.Digest != identity.Digest {
			t.Error("renderer changed the captured source or range sequence")
		}
		return identity.Read(source, request)
	})
	var body strings.Builder
	var final *plugin.DocumentIdentity
	chunks := 0
	for event, err := range client.method.StreamWithMetadata(t.Context(), client.binding, client.callbacks, pluginmcp.CallbackLimits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20}, "", identity, metadata) {
		if err != nil {
			t.Fatal(err)
		}
		if event.Chunk != nil {
			if final != nil || !utf8.ValidString(*event.Chunk) {
				t.Fatal("renderer emitted out-of-order or broken UTF-8 evidence")
			}
			if err := plugin.ValidateRenderedChunk(body.Len(), len(*event.Chunk)); err != nil {
				t.Fatal(err)
			}
			body.WriteString(*event.Chunk)
			chunks++
		} else {
			if final != nil {
				t.Fatal("renderer emitted a second final identity")
			}
			final = event.Result
		}
	}
	if final == nil || *final != plugin.IdentifyDocument([]byte(body.String())) || chunks < 2 || reads.Load() < 2 {
		t.Fatalf("complete stream identity/ranges lost: final=%+v chunks=%d reads=%d", final, chunks, reads.Load())
	}
	for _, evidence := range []string{fullText, `"revision": 9007199254740993`, "9007199254740995", "1.2300", "1e+100", "-0", "recorded execution failure", `"status": "incomplete"`, "```json\nnull\n```", "No result was recorded.", "### Model-visible text evidence\n\n```text\nunique diagnostic not present in result\n```"} {
		if !strings.Contains(body.String(), evidence) {
			t.Errorf("stream dropped original evidence %q", evidence)
		}
	}
	if strings.Count(body.String(), "No result was recorded.") != 1 || strings.Count(body.String(), fullText) != 1 {
		t.Fatal("stream collapsed null into absence or duplicated its original text")
	}
	if strings.Count(body.String(), "unique diagnostic not present in result") != 1 {
		t.Fatal("stream lost or duplicated the separate model-visible evidence")
	}
}

func TestMarkdownStreamRefusesInvalidSourceBeforeEmittingFragments(t *testing.T) {
	client := openMarkdownClient(t)
	for _, fixture := range []struct {
		name    string
		version int
		item    protocol.ArtifactItem
		results []protocol.ArtifactToolResult
		want    string
	}{
		{"missing invocation", protocol.SessionArtifactVersion, protocol.ArtifactItem{Type: protocol.ItemTypeToolCall}, nil, "tool invocation is absent"},
		{"former version", protocol.SessionArtifactVersion - 1, protocol.ArtifactItem{Type: protocol.ItemTypeUserMessage, Text: "old source"}, nil, "unsupported archive version"},
		{"unmatched model text", protocol.SessionArtifactVersion, protocol.ArtifactItem{ID: "user-item", Type: protocol.ItemTypeUserMessage, Text: "input"}, []protocol.ArtifactToolResult{{ID: "model-text", ItemID: "absent-tool", ModelOnly: true, Body: "original text"}}, "no matching tool item"},
		{"duplicate model text", protocol.SessionArtifactVersion, protocol.ArtifactItem{ID: "tool-item", Type: protocol.ItemTypeToolCall, Tool: &protocol.ToolInvocation{Name: "inspect", Arguments: map[string]any{}, Result: jsontext.Value(`{"revision":7}`)}}, []protocol.ArtifactToolResult{
			{ID: "first", ItemID: "tool-item", ModelOnly: true, Body: "first original text"},
			{ID: "second", ItemID: "tool-item", ModelOnly: true, Body: "second original text"},
		}, "duplicate model-visible text evidence"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			source, err := json.Marshal(protocol.SessionArtifact{Version: fixture.version, Session: protocol.ArtifactSession{ID: "session"}, Items: []protocol.ArtifactItem{fixture.item}, ToolResults: fixture.results})
			if err != nil {
				t.Fatal(err)
			}
			identity := plugin.IdentifyDocument(source)
			metadata := client.metadata(t, func(request plugin.DocumentRange) (string, error) { return identity.Read(source, request) })
			var failure error
			for event, err := range client.method.StreamWithMetadata(t.Context(), client.binding, client.callbacks, pluginmcp.CallbackLimits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20}, "", identity, metadata) {
				if event.Chunk != nil || event.Result != nil {
					t.Fatal("invalid source was published as a successful fragment or document")
				}
				failure = err
			}
			if failure == nil || !strings.Contains(failure.Error(), fixture.want) {
				t.Fatalf("source refusal lost its cause: %v", failure)
			}
		})
	}
}
