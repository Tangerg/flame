package http_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestRPCRefusesRepeatedScalarHeadersBeforeDispatch(t *testing.T) {
	for _, test := range []struct{ header, value, key string }{
		{header: "Authorization", value: "Bearer original"},
		{header: "Content-Type", value: "application/json"},
		{header: "Idempotency-Key", value: "original"},
		{header: "Idempotency-Namespace", value: testsupport.IdempotencyNamespace, key: "original"},
		{header: "Last-Event-Id", value: "evt_original"},
		{header: "X-Method", value: "runs.cancel"},
	} {
		t.Run(test.header, func(t *testing.T) {
			server, api := newTestServer(t)
			defer server.Close()
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/v2/rpc", bytes.NewBufferString(`{"jsonrpc":"2.0","id":"1","method":"runs.cancel","params":{"runId":"run_original"}}`))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			if test.key != "" {
				request.Header.Set("Idempotency-Key", test.key)
			}
			request.Header.Set(test.header, test.value)
			request.Header.Add(test.header, test.value)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", response.StatusCode)
			}
			if len(api.canceledRuns) != 0 {
				t.Errorf("ambiguous request canceled runs: %v", api.canceledRuns)
			}
		})
	}
}
