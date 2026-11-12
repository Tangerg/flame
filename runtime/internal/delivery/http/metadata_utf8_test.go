package http_test

import (
	"bytes"
	"errors"
	"net/http"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestRPCRejectsMetadataThatCannotRetainJSONIdentity(t *testing.T) {
	server, api := newTestServer(t)
	defer server.Close()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/v2/rpc", bytes.NewBufferString(`{"jsonrpc":"2.0","id":"1","method":"runs.cancel","params":{"runId":"run_original"}}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "original\xff")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if len(api.canceledRuns) != 0 {
		t.Errorf("unrecoverable command identity reached business execution: %v", api.canceledRuns)
	}
	if code := decodeErrorCode(t, response); code != -32602 {
		t.Errorf("code = %d, want invalid_params", code)
	}
}

func TestBindingKeepsReceiptKeysRepresentableWithoutRestrictingUnicode(t *testing.T) {
	for _, test := range []struct {
		key   string
		valid bool
	}{{key: "original\xff"}, {key: "原始身份", valid: true}} {
		t.Run(test.key, func(t *testing.T) {
			api := &fakeRuntime{}
			endpoint := newTestEndpoint(t, api, delivery.EndpointConfig{IdempotencyNamespace: testsupport.IdempotencyNamespace})
			result := endpoint.Invoke(t.Context(), delivery.RunsCancel, protocol.CancelRunRequest{RunID: "run_original"}, delivery.Options{IdempotencyKey: test.key})
			if test.valid {
				if result.Failure != nil || len(api.canceledRuns) != 1 {
					t.Fatalf("representable identity failed: %v; canceled runs %v", result.Failure, api.canceledRuns)
				}
				return
			}
			if result.Failure == nil || !errors.Is(result.Failure, protocol.ErrInvalidParams) || len(api.canceledRuns) != 0 {
				t.Fatalf("unrepresentable identity advanced: %v; canceled runs %v", result.Failure, api.canceledRuns)
			}
		})
	}
}
