package main

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync/atomic"
	"testing"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend/contract"

	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestReceiptHistoryRuntimeFixture(t *testing.T) {
	directory := os.Getenv("FLAME_TEST_REVIEWS_RUNTIME_DATA")
	if directory == "" {
		return
	}
	store := openTestStore(t, directory)
	initial, err := store.list(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	input := reviews.Update{ID: "history-missing", ExpectedRevision: 1, Status: reviews.Open}
	admitted, err := jsonvalue.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	// Retain the former cutoff through real business calls before publishing the
	// endpoint, so history setup cannot consume Runtime's handshake window.
	for index := range 4096 {
		result, err := store.update(t.Context(), fmt.Sprintf("runtime-history-%d", index), input, admitted)
		if err != nil || result.Type != reviews.NotFound {
			t.Fatalf("prepare receipt history: %+v, %v", result, err)
		}
	}
	server, err := newServer(store)
	if err != nil {
		t.Fatal(err)
	}
	contracts, err := reviews.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	var updates, inputChecks, toolUpdates, receiptReads atomic.Int32
	server.AddReceivingMiddleware(func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, request sdk.Request) (sdk.Result, error) {
			if call, ok := request.(*sdk.CallToolRequest); ok && call.Params != nil && call.Params.Name == "update_review" {
				toolUpdates.Add(1)
			}
			var operation struct {
				Method    string `json:"method"`
				CommandID string `json:"commandId"`
			}
			if sdk.HasParams(request) {
				encoded, err := json.Marshal(request.GetParams())
				if err != nil {
					return nil, err
				}
				if err := json.Unmarshal(encoded, &operation); err != nil {
					return nil, err
				}
			}
			switch operation.Method {
			case contracts.Update.Name():
				// Structural admission has no mutation authority or receipt identity.
				if operation.CommandID == "" {
					inputChecks.Add(1)
				} else {
					updates.Add(1)
				}
			case pluginsdk.ReceiptMethod:
				receiptReads.Add(1)
			}
			return next(ctx, method, request)
		}
	})
	remote := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil))
	defer remote.Close()
	fmt.Println(remote.URL)
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		t.Fatal(err)
	}
	current, err := store.list(t.Context())
	if err != nil || !reflect.DeepEqual(current, initial) {
		t.Fatalf("history refusal changed private review: %+v, %v", current, err)
	}
	var receipts int
	if err := store.db.QueryRowContext(t.Context(), `SELECT count(*) FROM invocations`).Scan(&receipts); err != nil || receipts != 4098 {
		t.Fatalf("new refusals were lost or recovery wrote receipts: %d, %v", receipts, err)
	}
	if updates.Load() != 1 || inputChecks.Load() != 1 || toolUpdates.Load() != 1 || receiptReads.Load() != 2 {
		t.Fatalf("history recovery changed dispatch: typed=%d validation=%d tool=%d receipt=%d", updates.Load(), inputChecks.Load(), toolUpdates.Load(), receiptReads.Load())
	}
}
