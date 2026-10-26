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

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	rpc "github.com/Tangerg/flame/plugins/knowledgebase/internal/contract"
	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/internal/domain"
	sdk "github.com/Tangerg/go-sdk/mcp"
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
	if err := pluginsdk.RegisterClient(client); err != nil {
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
	host := pluginsdk.NewCallbacks()
	t.Cleanup(func() { _ = host.Close() })
	host.AdmitSession(session)
	var space atomic.Value
	space.Store("space-a")
	var calls atomic.Int32
	embedding, err := pluginsdk.NewTextEmbeddingCallback(func(ctx context.Context, command pluginsdk.Command[[]string]) (pluginsdk.TextEmbeddings, error) {
		owner, err := pluginsdk.InvocationFromContext(ctx)
		if err != nil || owner.ID != command.ID || owner.Parent != "knowledge-entry" {
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
	owner, err := pluginsdk.WithInvocation(t.Context(), extension.Invocation{ID: "knowledge-entry", Origin: extension.Human, Target: extension.Target{Scope: "installation", Reference: "knowledge"}})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := host.Open(owner, session, pluginsdk.CallbackLimits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20}, embedding)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	metadata, err := pluginsdk.CallbackMetadata(ref)
	if err != nil {
		t.Fatal(err)
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	exported, err := pluginsdk.ResolveCapability(t.Context(), session, "knowledgebase", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	importMethod, err := exported.Method(contracts.Import.Name())
	if err != nil {
		t.Fatal(err)
	}
	input := rpc.ImportInput{Content: knowledge.Content{ID: "fruit", Title: "Fruit guide", Source: "docs/fruit.md", Text: "apple apple apple"}}
	admitted, err := jsonvalue.Encode(input)
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
	query := rpc.SearchInput{Query: "apple", Limit: 2}
	queryJSON, err := jsonvalue.Encode(query)
	if err != nil {
		t.Fatal(err)
	}
	typed, err := searchMethod.InvokeWithMetadata(owner, "", queryJSON, metadata)
	if err != nil {
		t.Fatal(err)
	}
	toolMeta := maps.Clone(metadata.Meta)
	if err := toolMeta.Set(pluginsdk.InvocationMetadataKey, extension.Invocation{ID: "model-search", Origin: extension.Model, Target: extension.Target{Scope: "installation", Reference: "knowledge"}}); err != nil {
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
	toolJSON, err := jsonvalue.Parse(encoded)
	if err != nil || toolJSON != typed {
		t.Fatalf("typed and Tool retrieval diverged: %s, %s, %v", typed.String(), toolJSON.String(), err)
	}
	contextContract, err := pluginsdk.ModelContextContract()
	if err != nil {
		t.Fatal(err)
	}
	contextExport, err := pluginsdk.ResolveCapability(t.Context(), session, "context", contextContract)
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
	contextInput, err := jsonvalue.Encode(request)
	if err != nil {
		t.Fatal(err)
	}
	contextOwner, err := pluginsdk.WithInvocation(t.Context(), extension.Invocation{ID: "context-entry", Origin: extension.Model, Target: extension.Target{Scope: "call", Reference: "context-entry"}})
	if err != nil {
		t.Fatal(err)
	}
	contribution, err := contextBinding.InvokeWithMetadata(contextOwner, "", contextInput, metadata)
	if err != nil {
		t.Fatal(err)
	}
	var contextResult pluginsdk.ModelContextResult
	if err := json.Unmarshal(contribution.JSON(), &contextResult); err != nil {
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
	} else if refusal, ok := errors.AsType[*extension.Refusal](err); !ok || refusal.Code != "embedding_space_changed" {
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
	deleteInput, err := jsonvalue.Encode(rpc.DocumentInput{ID: "fruit", ExpectedRevision: 1})
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
