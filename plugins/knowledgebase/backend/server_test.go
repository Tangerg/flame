package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"maps"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/frame/portable"
	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	rpc "github.com/Tangerg/flame/plugins/knowledgebase/internal/contract"
	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/internal/domain"
	"github.com/Tangerg/flame/plugins/knowledgebase/internal/wire"
	sdk "github.com/Tangerg/go-sdk/mcp"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"

	"github.com/Tangerg/scope/core/chat"
)

func TestTypedToolAndContextEntriesRetrieveThroughTheSameHostEmbedding(t *testing.T) {
	store, err := openStore(t.Context(), filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.db.Close() })
	library, err := newLibrary(store, remoteEmbedding)
	if err != nil {
		t.Fatal(err)
	}
	server, err := newServer(library)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "knowledge-test"}, nil)
	if err := pluginmcp.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	a, b := sdk.NewInMemoryTransports()
	backend, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	host := pluginmcp.NewCallbacks()
	t.Cleanup(func() { _ = host.Close() })
	if err := host.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	var space atomic.Value
	space.Store("space-a")
	var calls atomic.Int32
	embedding, err := pluginsdk.NewTextEmbeddingCallback(func(ctx context.Context, command pluginmcp.Command[[]string]) (pluginsdk.TextEmbeddings, error) {
		owner, err := pluginmcp.InvocationFromContext(ctx)
		if err != nil || owner.ID != string(command.ID) || owner.Parent != "knowledge-entry" {
			t.Errorf("embedding lost calling owner: %+v, %v", owner, err)
		}
		calls.Add(1)
		vectors := make([][]float32, len(command.Input))
		for i, text := range command.Input {
			vectors[i] = []float32{float32(strings.Count(text, "apple")), 1}
		}
		return pluginsdk.TextEmbeddings{Space: space.Load().(string), Vectors: vectors}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := pluginmcp.WithInvocation(t.Context(), framemcp.Invocation{ID: "knowledge-entry", Origin: extension.Human, Target: portable.Target{Scope: "installation", Reference: "knowledge"}})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := host.Open(owner, session, pluginmcp.CallbackLimits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20}, embedding)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	metadata, err := pluginmcp.CallbackMetadata(ref)
	if err != nil {
		t.Fatal(err)
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	exported, err := pluginmcp.ResolveCapability(t.Context(), session, "knowledgebase", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	importMethod, err := exported.Method(contracts.Import.Name())
	if err != nil {
		t.Fatal(err)
	}
	input := wire.ImportInput{Content: knowledge.Content{ID: "fruit", Title: "Fruit guide", Source: "docs/fruit.md", Text: "apple apple apple"}}
	admitted, err := framevalue.EncodeJSON(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importMethod.InvokeWithMetadata(owner, "import-original", admitted, metadata); err != nil {
		t.Fatal(err)
	}
	searchMethod, err := exported.Method(contracts.Search.Name())
	if err != nil {
		t.Fatal(err)
	}
	query := wire.SearchInput{Query: "apple", Limit: 2}
	queryJSON, err := framevalue.EncodeJSON(query)
	if err != nil {
		t.Fatal(err)
	}
	typed, err := searchMethod.InvokeWithMetadata(owner, "", queryJSON, metadata)
	if err != nil {
		t.Fatal(err)
	}
	toolMeta := maps.Clone(metadata.Meta)
	if err := toolMeta.Set(pluginmcp.InvocationMetadataKey, framemcp.Invocation{ID: "model-search", Origin: extension.Model, Target: portable.Target{Scope: "installation", Reference: "knowledge"}}); err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "search_knowledge", Arguments: query, Meta: toolMeta})
	if err != nil || result.IsError {
		t.Fatalf("model retrieval = %+v, %v", result, err)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	toolJSON, err := framevalue.ParseJSON(encoded)
	if err != nil || toolJSON != typed {
		t.Fatalf("typed and Tool retrieval diverged: %s, %s, %v", typed.String(), toolJSON.String(), err)
	}
	readMethod, err := contracts.Read.Bind(exported)
	if err != nil {
		t.Fatal(err)
	}
	beforeRead := calls.Load()
	read, err := readMethod.Call(t.Context(), wire.ReadInput{ID: input.Content.ID})
	if err != nil || read.Text != input.Content.Text || read.Document.Revision != 1 {
		t.Fatalf("typed original read: %+v %v", read, err)
	}
	var readMeta sdk.Meta
	if err := readMeta.Set(pluginmcp.InvocationMetadataKey, framemcp.Invocation{ID: "model-read", Origin: extension.Model, Target: portable.Target{Scope: "installation", Reference: "knowledge"}}); err != nil {
		t.Fatal(err)
	}
	readResult, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "read_knowledge_document", Arguments: wire.ReadInput{ID: input.Content.ID}, Meta: readMeta})
	if err != nil || readResult.IsError {
		t.Fatalf("model original read without an embedding callback: %+v %v", readResult, err)
	}
	encodedRead, err := json.Marshal(readResult.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var modelRead wire.DocumentRead
	if err := json.Unmarshal(encodedRead, &modelRead); err != nil || modelRead != read || calls.Load() != beforeRead {
		t.Fatalf("model and typed reads diverged or performed embedding: %+v calls=%d %v", modelRead, calls.Load(), err)
	}
	contextContract, err := pluginsdk.ModelContextContract()
	if err != nil {
		t.Fatal(err)
	}
	contextExport, err := pluginmcp.ResolveCapability(t.Context(), session, "context", contextContract)
	if err != nil {
		t.Fatal(err)
	}
	contextMethod, err := pluginsdk.ModelContextMethod()
	if err != nil {
		t.Fatal(err)
	}
	contextBinding, err := contextExport.Method(contextMethod.Name())
	if err != nil {
		t.Fatal(err)
	}
	request := pluginsdk.ModelContextRequest{Provider: "test", Model: "test", Request: chat.Request{Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("apple"))}}}
	request.UserMessage = new(request.Request.Messages[0].Clone())
	request.Request.Messages = append(request.Request.Messages, chat.NewUserMessage(chat.NewTextPart("host-owned catalog tail")))
	contextInput, err := framevalue.EncodeJSON(request)
	if err != nil {
		t.Fatal(err)
	}
	contextOwner, err := pluginmcp.WithInvocation(t.Context(), framemcp.Invocation{ID: "context-entry", Origin: extension.Model, Target: portable.Target{Scope: "call", Reference: "context-entry"}})
	if err != nil {
		t.Fatal(err)
	}
	contribution, err := contextBinding.InvokeWithMetadata(contextOwner, "", contextInput, metadata)
	if err != nil {
		t.Fatal(err)
	}
	var contextResult pluginsdk.ModelContextResult
	if err := json.Unmarshal(contribution.Bytes(), &contextResult); err != nil {
		t.Fatal(err)
	}
	if err := contextResult.Validate(); err != nil || len(contextResult.Messages) != 1 || !strings.Contains(contextResult.Messages[0].Text(), "docs/fruit.md") {
		t.Fatalf("context lost citation or changed conversation roles: %+v, %v", contextResult, err)
	}
	if calls.Load() != 4 {
		t.Fatalf("unexpected embedding attempts: %d", calls.Load())
	}
	space.Store("space-b")
	if _, err := searchMethod.InvokeWithMetadata(owner, "", queryJSON, metadata); err == nil {
		t.Fatal("changed space became a successful retrieval")
	} else if refusal, ok := errors.AsType[*framemcp.Refusal](err); !ok || refusal.Code != "embedding_space_changed" {
		t.Fatalf("changed space refusal = %v", err)
	}
	result, err = session.CallTool(t.Context(), &sdk.CallToolParams{Name: "search_knowledge", Arguments: query, Meta: toolMeta})
	if err != nil || !result.IsError {
		t.Fatalf("Tool space mismatch lost its known refusal: %+v, %v", result, err)
	}
	deleteMethod, err := exported.Method(contracts.Delete.Name())
	if err != nil {
		t.Fatal(err)
	}
	deleteInput, err := framevalue.EncodeJSON(wire.DocumentInput{ID: "fruit", ExpectedRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deleteMethod.Invoke(owner, "delete-original", deleteInput); err != nil {
		t.Fatal(err)
	}
	if _, err := importMethod.InvokeWithMetadata(owner, "reimport", admitted, metadata); err == nil {
		t.Fatal("deleted identity admitted an old revision again")
	}
}
