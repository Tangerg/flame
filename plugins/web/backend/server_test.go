package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	"github.com/Tangerg/flame/plugins/web/backend/domain"
	"github.com/Tangerg/flame/plugins/web/backend/rpc"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/tools/web"
)

func TestTypedAndModelToolsShareSearchFetchBrowseAndDefiniteReadFailure(t *testing.T) {
	store, err := openStore(t.Context(), filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.db.Close()
	fail := false
	research, err := newResearch(store, func(context.Context, *web.FetchRequest) (*web.FetchResponse, error) {
		if fail {
			return nil, errors.New("upstream unavailable")
		}
		return &web.FetchResponse{Content: "captured content", Format: web.FormatMarkdown}, nil
	}, func(_ context.Context, request *web.SearchRequest) (*web.SearchResponse, error) {
		return &web.SearchResponse{Query: request.Query, Results: []*web.SearchResult{{Title: "Example", URL: "https://example.test", Snippet: "source"}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	server, err := newServer(research)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "web-test", Version: "1"}, nil)
	if err := pluginsdk.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	a, b := sdk.NewInMemoryTransports()
	connection, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	exported, err := pluginsdk.ResolveCapability(t.Context(), session, "web", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "web-entry", Origin: capability.Human, Target: capability.Target{Scope: "installation", Reference: "web"}})
	if err != nil {
		t.Fatal(err)
	}
	meta := make(sdk.Meta)
	if err := meta.Set(pluginsdk.InvocationMetadataKey, capability.Invocation{ID: "model-web-entry", Origin: capability.Model, Target: capability.Target{Scope: "installation", Reference: "web"}}); err != nil {
		t.Fatal(err)
	}
	compare := func(method, name string, input any) jsoninput.Input {
		t.Helper()
		bound, err := exported.Method(method)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := jsoninput.Encode(input)
		if err != nil {
			t.Fatal(err)
		}
		typed, err := bound.Invoke(ctx, "", encoded)
		if err != nil {
			t.Fatal(err)
		}
		model, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: name, Arguments: input, Meta: meta})
		if err != nil || model.IsError {
			t.Fatalf("model read: %+v %v", model, err)
		}
		bytes, err := json.Marshal(model.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		projected, err := jsoninput.Parse(bytes)
		if err != nil || projected != typed {
			t.Fatalf("typed/model read diverged: %s %s %v", typed.String(), projected.String(), err)
		}
		return typed
	}
	compare(contracts.Search.Name(), "web_search", web.SearchRequest{Query: "query", MaxResults: 1})
	fetched := compare(contracts.Fetch.Name(), "web_fetch", web.FetchRequest{URL: "https://example.test"})
	var page domain.Page
	if err := json.Unmarshal(fetched.JSON(), &page); err != nil {
		t.Fatal(err)
	}
	compare(contracts.Browse.Name(), "web_browse", rpc.BrowseInput{SnapshotID: page.SnapshotID, Limit: 8192})
	fail = true
	failed, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "web_fetch", Arguments: web.FetchRequest{URL: "https://example.test"}, Meta: meta})
	if err != nil || !failed.IsError {
		t.Fatalf("failed read became an unknown protocol outcome: %+v %v", failed, err)
	}
	bytes, err := json.Marshal(failed)
	if err != nil || !strings.Contains(string(bytes), "web: read failed") {
		t.Fatalf("failed read output: %s %v", bytes, err)
	}
}
