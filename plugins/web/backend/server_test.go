package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"

	rpc "github.com/Tangerg/flame/plugins/web/internal/contract"
	capture "github.com/Tangerg/flame/plugins/web/internal/domain"
	"github.com/Tangerg/flame/plugins/web/internal/wire"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/tools/httpreq"
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
	requestCount := 0
	requests, err := newRequests(store.db, httpreq.ClientConfig{AllowedHosts: []string{"api.test"}, HTTPClient: &http.Client{Transport: httpTransport(func(*http.Request) (*http.Response, error) {
		requestCount++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("original HTTP response"))}, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	server, err := newServer(research, requests)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "web-test", Version: "1"}, nil)
	if err := qimumcp.RegisterClient(client); err != nil {
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
	exported, err := qimumcp.ResolveCapability(t.Context(), session, "web", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: "web-entry", Origin: extension.Human, Target: portable.Target{Scope: "installation", Reference: "web"}})
	if err != nil {
		t.Fatal(err)
	}
	meta := make(sdk.Meta)
	if err := meta.Set(qimumcp.InvocationMetadataKey, portable.Invocation{ID: "model-web-entry", Origin: extension.Model, Target: portable.Target{Scope: "installation", Reference: "web"}}); err != nil {
		t.Fatal(err)
	}
	compare := func(method, name string, input any) jsonvalue.Value {
		t.Helper()
		bound, err := exported.Method(method)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := jsonvalue.Encode(input)
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
		projected, err := jsonvalue.Parse(bytes)
		if err != nil || projected != typed {
			t.Fatalf("typed/model read diverged: %s %s %v", typed.String(), projected.String(), err)
		}
		return typed
	}
	compare(contracts.Search.Name(), "web_search", web.SearchRequest{Query: "query", MaxResults: 1})
	fetched := compare(contracts.Fetch.Name(), "web_fetch", web.FetchRequest{URL: "https://example.test"})
	var page capture.Page
	if err := json.Unmarshal(fetched.JSON(), &page); err != nil {
		t.Fatal(err)
	}
	compare(contracts.Browse.Name(), "web_browse", wire.BrowseInput{SnapshotID: page.SnapshotID, Limit: 8192})
	httpInput := httpreq.Request{URL: "https://api.test/resource"}
	httpRequest, err := jsonvalue.Encode(httpInput)
	if err != nil {
		t.Fatal(err)
	}
	httpMethod, err := exported.Method(contracts.Request.Name())
	if err != nil {
		t.Fatal(err)
	}
	typedHTTP, err := httpMethod.Invoke(ctx, "http-command", httpRequest)
	if err != nil {
		t.Fatal(err)
	}
	modelHTTP, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "http_request", Arguments: httpInput, Meta: meta})
	if err != nil || modelHTTP.IsError {
		t.Fatalf("HTTP tool: %+v %v", modelHTTP, err)
	}
	var typedResponse, modelResponse wire.HTTPResponse
	if err := json.Unmarshal(typedHTTP.JSON(), &typedResponse); err != nil {
		t.Fatal(err)
	}
	encodedHTTP, err := json.Marshal(modelHTTP.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encodedHTTP, &modelResponse); err != nil {
		t.Fatal(err)
	}
	if typedResponse.Status != modelResponse.Status || typedResponse.Body != modelResponse.Body || requestCount != 2 {
		t.Fatalf("HTTP result parity: %+v %+v calls=%d", typedResponse, modelResponse, requestCount)
	}
	if _, err := httpMethod.Invoke(ctx, "http-command", httpRequest); err != nil || requestCount != 2 {
		t.Fatalf("typed HTTP replay: %v calls=%d", err, requestCount)
	}
	receipt, err := pluginsdk.ReadToolReceipt(ctx, session, "model-web-entry", "http_request", httpRequest)
	if err != nil || receipt == nil || requestCount != 2 {
		t.Fatalf("HTTP Tool recovery: %+v %v calls=%d", receipt, err, requestCount)
	}
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
