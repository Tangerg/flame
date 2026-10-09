package reviews

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestMCPContractOwnsInputsResultsAndInvocationReceipts(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	server, err := NewServer(store)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil))
	t.Cleanup(httpServer.Close)
	client := sdk.NewClient(&sdk.Implementation{Name: "review-test", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), &sdk.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	})
	catalog, err := session.ListTools(t.Context(), nil)
	if err != nil || len(catalog.Tools) != 2 {
		t.Fatalf("catalog = %+v, %v", catalog, err)
	}
	call := func(id string, args any) *sdk.CallToolResult {
		t.Helper()
		meta := sdk.Meta{}
		if id != "" {
			meta[InvocationMetadataKey] = id
		}
		result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "update_review", Arguments: args, Meta: meta})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	input := Update{ID: ExampleReviewID, ExpectedRevision: 1, Status: Resolved}
	if result := call("", input); !result.IsError {
		t.Fatalf("missing identity = %+v", result)
	}
	for _, args := range []any{
		map[string]any{"id": ExampleReviewID, "status": Resolved},
		map[string]any{"id": ExampleReviewID, "expectedRevision": nil, "status": Resolved},
		map[string]any{"id": ExampleReviewID, "expectedRevision": 1, "status": Resolved, "unexpected": true},
	} {
		if result := call("bad", args); !result.IsError {
			t.Fatalf("invalid arguments = %+v", result)
		}
	}
	accepted := call("original", input)
	if accepted.IsError {
		t.Fatalf("accepted = %+v", accepted)
	}
	encoded, err := json.Marshal(accepted.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var result UpdateResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result.Type != Updated || result.Review == nil || result.Review.Revision != 2 {
		t.Fatalf("result = %+v", result)
	}
	replayed := call("original", input)
	encodedReplay, err := json.Marshal(replayed.StructuredContent)
	if err != nil || string(encodedReplay) != string(encoded) || replayed.IsError {
		t.Fatalf("replay = %+v, %v", replayed, err)
	}
	if stale := call("new", input); !stale.IsError {
		t.Fatalf("stale revision = %+v", stale)
	}
	listed, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "list_reviews", Arguments: map[string]any{}})
	if err != nil || listed.IsError {
		t.Fatalf("list = %+v, %v", listed, err)
	}
	var current struct {
		Reviews []Snapshot `json:"reviews"`
	}
	encoded, err = json.Marshal(listed.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &current); err != nil {
		t.Fatal(err)
	}
	if len(current.Reviews) != 1 || current.Reviews[0].Revision != 2 || current.Reviews[0].Status != Resolved {
		t.Fatalf("current = %+v", current)
	}
}

func TestUnexpectedStoreFailureRemainsProtocolFailure(t *testing.T) {
	store, err := OpenStore(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := sdk.NewClient(&sdk.Implementation{Name: "failure-test", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "update_review", Arguments: Update{ID: ExampleReviewID, ExpectedRevision: 1, Status: Resolved}, Meta: sdk.Meta{InvocationMetadataKey: "original"}})
	if err == nil || result != nil {
		t.Fatalf("unexpected store failure became a definite tool result: %+v, %v", result, err)
	}
}
