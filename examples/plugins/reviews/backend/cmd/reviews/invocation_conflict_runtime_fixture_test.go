package main

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"github.com/Tangerg/flame/qimu/portable"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend/contract"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestInvocationConflictRuntimeFixture(t *testing.T) {
	directory := os.Getenv("FLAME_TEST_REVIEWS_RUNTIME_DATA")
	if directory == "" {
		return
	}
	store := openTestStore(t, directory)
	initial, err := store.list(t.Context())
	if err != nil || len(initial) != 1 {
		t.Fatalf("read initial review: %+v, %v", initial, err)
	}
	prior := reviews.Update{ID: "prior-private-call", ExpectedRevision: 1, Status: reviews.Open}
	admitted, err := jsonvalue.Encode(prior)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var priorIDs []string
	claim := func(ctx context.Context, id string) error {
		// A real prior private call owns the identity before the unchanged public
		// handler sees different input; no fabricated response or receipt is used.
		result, err := store.update(ctx, id, admitted)
		if err != nil {
			return fmt.Errorf("prepare prior private binding: %w", err)
		}
		if result != (reviews.UpdateResult{Type: reviews.NotFound}) {
			return fmt.Errorf("prepare prior private binding: unexpected outcome %+v", result)
		}
		mu.Lock()
		priorIDs = append(priorIDs, id)
		mu.Unlock()
		return nil
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
				if toolUpdates.Add(1) == 1 {
					var invocation portable.Invocation
					if err := json.Unmarshal(call.Params.Meta[qimumcp.InvocationMetadataKey], &invocation, json.RejectUnknownMembers(true)); err != nil {
						return nil, err
					}
					if err := invocation.Validate(); err != nil {
						return nil, err
					}
					if err := claim(ctx, invocation.ID); err != nil {
						return nil, err
					}
				}
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
				if operation.CommandID == "" {
					inputChecks.Add(1)
				} else if updates.Add(1) == 1 {
					if err := claim(ctx, operation.CommandID); err != nil {
						return nil, err
					}
				}
			case qimumcp.ReceiptMethod:
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
	remote.Close()
	expected := initial[0]
	expected.Revision, expected.Status = 2, reviews.Resolved
	current, err := store.list(t.Context())
	if err != nil || !reflect.DeepEqual(current, []reviews.Snapshot{expected}) {
		t.Fatalf("conflict recovery or fresh command changed private state: %+v, %v", current, err)
	}
	var receipts int
	if err := store.db.QueryRowContext(t.Context(), `SELECT count(*) FROM invocations`).Scan(&receipts); err != nil || receipts != 3 {
		t.Fatalf("conflicts replaced or invented private receipts: %d, %v", receipts, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(priorIDs) != 2 || priorIDs[0] == priorIDs[1] {
		t.Fatalf("expected distinct prior Tool and typed identities: %v", priorIDs)
	}
	for _, id := range priorIDs {
		receipt, err := store.receipt(t.Context(), qimumcp.ReceiptRequest{ID: id, Method: contracts.Update.Name(), InputDigest: admitted.Digest()}, contracts.Update.Name())
		if err != nil || !receipt.Found {
			t.Fatalf("original private receipt lost: %+v, %v", receipt, err)
		}
		result, err := qimumcp.DecodeReply[reviews.UpdateResult](receipt.Reply)
		if err != nil || result != (reviews.UpdateResult{Type: reviews.NotFound}) {
			t.Fatalf("conflict overwrote original private outcome: %+v, %v", result, err)
		}
	}
	if updates.Load() != 2 || inputChecks.Load() != 2 || toolUpdates.Load() != 1 || receiptReads.Load() != 2 {
		t.Fatalf("conflict recovery changed dispatch: typed=%d validation=%d tool=%d receipt=%d", updates.Load(), inputChecks.Load(), toolUpdates.Load(), receiptReads.Load())
	}
}
