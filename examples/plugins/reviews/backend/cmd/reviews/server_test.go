package main

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend/contract"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func modelToolMetadata(t *testing.T, id string) sdk.Meta {
	t.Helper()
	var meta sdk.Meta
	if id != "" {
		if err := meta.Set(qimumcp.InvocationMetadataKey, portable.Invocation{ID: id, Origin: extension.Model, Target: portable.Target{Scope: "installation", Reference: "reviews-owner"}}); err != nil {
			t.Fatal(err)
		}
	}
	return meta
}

func TestMCPContractOwnsInputsResultsAndInvocationReceipts(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	server, err := newServer(store)
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
	if err != nil || len(catalog.Tools) != 3 {
		t.Fatalf("catalog = %+v, %v", catalog, err)
	}
	call := func(id string, args any) *sdk.CallToolResult {
		t.Helper()
		meta := modelToolMetadata(t, id)
		result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "update_review", Arguments: args, Meta: meta})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	input := reviews.Update{ID: reviews.ExampleReviewID, ExpectedRevision: 1, Status: reviews.Resolved}
	if result := call("", input); !result.IsError {
		t.Fatalf("missing identity = %+v", result)
	}
	for _, args := range []any{
		map[string]any{"id": reviews.ExampleReviewID, "status": reviews.Resolved},
		map[string]any{"id": reviews.ExampleReviewID, "expectedRevision": nil, "status": reviews.Resolved},
		map[string]any{"id": reviews.ExampleReviewID, "expectedRevision": 1, "status": reviews.Resolved, "unexpected": true},
	} {
		if result := call("bad", args); !result.IsError {
			t.Fatalf("invalid arguments = %+v", result)
		}
	}
	accepted := call("original", input)
	if accepted.IsError {
		t.Fatalf("accepted = %+v", accepted)
	}
	encoded, err := json.Marshal(accepted.StructuredContent, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	var result reviews.UpdateResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result.Type != reviews.Updated || result.Review == nil || result.Review.Revision != 2 {
		t.Fatalf("result = %+v", result)
	}
	replayed := call("original", input)
	encodedReplay, err := json.Marshal(replayed.StructuredContent, json.Deterministic(true))
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
		Reviews []reviews.Snapshot `json:"reviews"`
	}
	encoded, err = json.Marshal(listed.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &current); err != nil {
		t.Fatal(err)
	}
	if len(current.Reviews) != 1 || current.Reviews[0].Revision != 2 || current.Reviews[0].Status != reviews.Resolved {
		t.Fatalf("current = %+v", current)
	}
}

func TestTypedAndToolEntrancesShareTheBusinessReceipt(t *testing.T) {
	contracts, err := reviews.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	store := openTestStore(t, t.TempDir())
	server, err := newServer(store)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "typed-review-client"}, nil)
	if err := qimumcp.RegisterClient(client); err != nil {
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
	exported, err := qimumcp.ResolveCapability(t.Context(), session, "reviews", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	updateMethod, err := contracts.Update.Bind(exported)
	if err != nil {
		t.Fatal(err)
	}
	listMethod, err := contracts.List.Bind(exported)
	if err != nil {
		t.Fatal(err)
	}
	input := reviews.Update{ID: reviews.ExampleReviewID, ExpectedRevision: 1, Status: reviews.Resolved}
	accepted, err := updateMethod.CallCommand(t.Context(), "shared-command", input)
	if err != nil || accepted.Type != reviews.Updated || accepted.Review == nil || accepted.Review.Revision != 2 {
		t.Fatalf("typed update = %+v, %v", accepted, err)
	}
	replayed, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "update_review", Arguments: input, Meta: modelToolMetadata(t, "shared-command")})
	if err != nil || replayed.IsError {
		t.Fatalf("Tool replay = %+v, %v", replayed, err)
	}
	encoded, err := json.Marshal(replayed.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var observed reviews.UpdateResult
	if err := json.Unmarshal(encoded, &observed); err != nil || observed.Type != accepted.Type || observed.Review == nil || *observed.Review != *accepted.Review {
		t.Fatalf("Tool entrance created a different business result: %+v, %v", observed, err)
	}
	conflict, err := updateMethod.CallCommand(t.Context(), "second-command", input)
	if err != nil || conflict.Type != reviews.Conflict || conflict.Review == nil || conflict.Review.Revision != 2 {
		t.Fatalf("typed business refusal lost its current-review evidence: %+v, %v", conflict, err)
	}
	changed := input
	changed.Status = reviews.Open
	identityConflict, err := updateMethod.CallCommand(t.Context(), "shared-command", changed)
	if err != nil || identityConflict.Type != reviews.IdentityConflict {
		t.Fatalf("identity conflict = %+v, %v", identityConflict, err)
	}
	assertToolOutcome := func(identity string, input reviews.Update, expected reviews.UpdateResult) {
		t.Helper()
		result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "update_review", Arguments: input, Meta: modelToolMetadata(t, identity)})
		if err != nil || result.IsError != (expected.Type != reviews.Updated) {
			t.Fatalf("Tool refusal classification: %+v, %v", result, err)
		}
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := jsonvalue.Parse(encoded)
		if err != nil {
			t.Fatal(err)
		}
		want, err := jsonvalue.Encode(expected)
		if err != nil || actual != want {
			t.Fatalf("Tool changed the typed business result: %s, want %s, %v", actual.JSON(), want.JSON(), err)
		}
	}
	assertToolOutcome("tool-stale", input, conflict)
	assertToolOutcome("shared-command", changed, identityConflict)
	for _, invalid := range []reviews.Update{
		{ID: reviews.ExampleReviewID, ExpectedRevision: 0, Status: reviews.Resolved},
		{ID: strings.Repeat("界", 43), ExpectedRevision: 1, Status: reviews.Resolved},
	} {
		identity := fmt.Sprintf("invalid-%s-%d", invalid.ID, invalid.ExpectedRevision)
		refused, err := updateMethod.CallCommand(t.Context(), identity, invalid)
		if err != nil || refused.Type != reviews.Invalid || refused.Review != nil {
			t.Fatalf("typed invalid-input refusal = %+v, %v", refused, err)
		}
		assertToolOutcome(identity, invalid, refused)
		recovered, found, err := updateMethod.ReadReceipt(t.Context(), identity, invalid)
		if err != nil || !found || !reflect.DeepEqual(recovered, refused) {
			t.Fatalf("typed invalid-input receipt = %+v, %t, %v", recovered, found, err)
		}
	}

	missing := reviews.Update{ID: "history-missing", ExpectedRevision: 1, Status: reviews.Open}
	admitted, err := jsonvalue.Encode(missing)
	if err != nil {
		t.Fatal(err)
	}
	var receipts int
	if err := store.db.QueryRowContext(t.Context(), `SELECT count(*) FROM invocations`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	for index := receipts; index < 4096; index++ {
		result, err := store.update(t.Context(), fmt.Sprintf("history-%d", index), missing, admitted)
		if err != nil || result.Type != reviews.NotFound {
			t.Fatalf("retain receipt history: %+v, %v", result, err)
		}
	}
	changed.ExpectedRevision = accepted.Review.Revision
	advanced, err := updateMethod.CallCommand(t.Context(), "typed-after-history", changed)
	if err != nil || advanced.Type != reviews.Updated || advanced.Review == nil || advanced.Review.Revision != 3 || advanced.Review.Status != reviews.Open {
		t.Fatalf("history exhausted the typed entrance: %+v, %v", advanced, err)
	}
	assertToolOutcome("typed-after-history", changed, advanced)
	refused, err := updateMethod.CallCommand(t.Context(), "refused-after-history", changed)
	if err != nil || refused.Type != reviews.Conflict || refused.Review == nil || *refused.Review != *advanced.Review {
		t.Fatalf("typed refusal after history lost its evidence: %+v, %v", refused, err)
	}
	assertToolOutcome("tool-refused-after-history", changed, refused)
	if recovered, err := updateMethod.CallCommand(t.Context(), "shared-command", input); err != nil || !reflect.DeepEqual(recovered, accepted) {
		t.Fatalf("history changed original typed recovery: %+v, %v", recovered, err)
	}
	assertToolOutcome("shared-command", input, accepted)
	listed, err := listMethod.Call(t.Context(), struct{}{})
	if err != nil || len(listed.Reviews) != 1 || listed.Reviews[0] != *advanced.Review {
		t.Fatalf("canonical typed read = %+v, %v", listed, err)
	}
	if err := store.db.QueryRowContext(t.Context(), `SELECT count(*) FROM invocations`).Scan(&receipts); err != nil || receipts != 4099 {
		t.Fatalf("new receipts were lost or recovery wrote duplicates: %d, %v", receipts, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = updateMethod.CallCommand(t.Context(), "unknown-command", changed)
	if err == nil {
		t.Fatal("storage failure succeeded")
	}
	if _, classified := errors.AsType[*portable.Refusal](err); classified {
		t.Fatalf("unexpected storage error became a known refusal: %v", err)
	}
}

func TestUnexpectedStoreFailureRemainsProtocolFailure(t *testing.T) {
	store, err := openStore(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server, err := newServer(store)
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
	result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "update_review", Arguments: reviews.Update{ID: reviews.ExampleReviewID, ExpectedRevision: 1, Status: reviews.Resolved}, Meta: modelToolMetadata(t, "original")})
	if err == nil || result != nil {
		t.Fatalf("unexpected store failure became a definite tool result: %+v, %v", result, err)
	}
}
