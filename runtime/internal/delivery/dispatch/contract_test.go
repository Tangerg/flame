package dispatch

import (
	"encoding/json"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/transport"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
)

func newOperationEndpoint(t *testing.T) *delivery.Endpoint {
	t.Helper()
	endpoint, err := delivery.NewEndpoint(&delivery.Handler{}, delivery.EndpointConfig{Lifetime: t.Context(), IdempotencyStore: testsupport.NewIdempotencyStore()})
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}

func TestUnknownMethodStaysUnknown(t *testing.T) {
	result := New(newOperationEndpoint(t)).Dispatch(t.Context(), &transport.Request{
		ID: testID("1"), Method: "runs.teleport", Params: json.RawMessage(`{}`),
	})
	if result.Response == nil {
		t.Fatal("unknown method returned no response")
	}
	rpcError, ok := result.Response.Error.(*transport.Error)
	if !ok {
		t.Fatalf("response error = %T", result.Response.Error)
	}
	var problem protocol.ProblemData
	if err := json.Unmarshal(rpcError.Data, &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Type != "method_not_found" {
		t.Fatalf("problem type = %q", problem.Type)
	}
}
