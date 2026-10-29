package dispatch

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/go-sdk/jsonrpc"
)

func testID(value string) jsonrpc.ID {
	id, _ := jsonrpc.MakeID(value)
	return id
}

func TestMetadataExtractionCannotNormalizeInvalidParameters(t *testing.T) {
	for _, raw := range []string{
		`{"_meta":{},"provider":"\ud800"}`,
		"{\"_meta\":{},\"provider\":\"\xff\"}",
		`{"_meta":{"ProtocolVersion":"ignored"},"provider":"deepseek"}`,
		`{"_meta":{},"provider":"deepseek","APIKEY":null}`,
	} {
		request := &jsonrpc.Request{ID: testID("strict"), Method: "providers.update", Params: jsontext.Value(raw)}
		failure := dispatchMetadataFailure(t, request)
		if failure == nil || failure.Code != codeInvalidParams {
			t.Fatalf("invalid parameters accepted: %q: %+v", raw, failure)
		}
	}
}

func TestExtractRequestMetaStripsTransportMember(t *testing.T) {
	req := &jsonrpc.Request{
		ID:     testID("1"),
		Method: "runs.cancel",
		Params: jsontext.Value(fmt.Sprintf(`{
			"_meta": {
				"protocolVersion": %q,
				"clientInfo": { "name": "cli", "version": "0.1.0" },
				"clientCapabilities": {
					"features": {},
					"interruptTypes": ["approval"],
					"excludedEphemeralEvents": ["item.delta"]
				}
			},
			"runId": "run_1"
		}`, protocol.Version)),
	}

	meta, rpcErr := extractRequestMeta(req)
	if rpcErr != nil {
		t.Fatalf("extractRequestMeta error = %v", rpcErr)
	}
	if meta.ProtocolVersion != protocol.Version {
		t.Fatalf("protocolVersion = %q", meta.ProtocolVersion)
	}
	if meta.ClientInfo == nil || meta.ClientInfo.Name != "cli" {
		t.Fatalf("clientInfo = %+v", meta.ClientInfo)
	}
	if meta.ClientCapabilities == nil || len(meta.ClientCapabilities.InterruptTypes) != 1 {
		t.Fatalf("clientCapabilities = %+v", meta.ClientCapabilities)
	}
	if string(req.Params) != `{"runId":"run_1"}` {
		t.Fatalf("stripped params = %s", string(req.Params))
	}
}

func TestExtractRequestMetaRejectsMalformedMeta(t *testing.T) {
	req := &jsonrpc.Request{
		ID:     testID("1"),
		Method: "runs.cancel",
		Params: jsontext.Value(`{"_meta":"bad","runId":"run_1"}`),
	}

	_, rpcErr := extractRequestMeta(req)
	if rpcErr == nil {
		t.Fatalf("expected invalid params error")
	}
	if rpcErr.Code != codeInvalidParams {
		t.Fatalf("code = %d, want %d", rpcErr.Code, codeInvalidParams)
	}
}

func TestExtractRequestMetaRejectsNullMeta(t *testing.T) {
	req := &jsonrpc.Request{
		ID:     testID("1"),
		Method: "runs.cancel",
		Params: jsontext.Value(`{"_meta":null,"runId":"run_1"}`),
	}

	_, rpcErr := extractRequestMeta(req)
	if rpcErr == nil {
		t.Fatalf("expected invalid params error")
	}
	if rpcErr.Code != codeInvalidParams {
		t.Fatalf("code = %d, want %d", rpcErr.Code, codeInvalidParams)
	}
	var problem protocol.ProblemData
	if err := json.Unmarshal(rpcErr.Data, &problem); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(problem.Errors, []protocol.FieldError{{Field: "$", Detail: "must not be null"}}) {
		t.Fatalf("null metadata lost its root field: %+v", problem)
	}
}

func TestDispatchRejectsUnsupportedProtocolVersion(t *testing.T) {
	for _, version := range []string{
		"2026-09-26",
		"2027-01-01", // a client newer than this build
		"1900-01-01",
		"not-a-date",
	} {
		t.Run(version, func(t *testing.T) {
			req := &jsonrpc.Request{
				ID:     testID("1"),
				Method: "runs.cancel",
				Params: jsontext.Value(fmt.Sprintf(`{"_meta":{"protocolVersion":%q},"runId":"run_1"}`, version)),
			}

			rpcErr := dispatchMetadataFailure(t, req)
			if rpcErr == nil {
				t.Fatalf("Dispatch accepted protocolVersion %q", version)
			}
			if rpcErr.Code != codeInvalidProtocolVersion {
				t.Fatalf("code = %d, want %d", rpcErr.Code, codeInvalidProtocolVersion)
			}
			// The refusal has to name the exact version: a client that only learns "no" cannot
			// tell an unsupported version from a malformed request.
			var problem struct {
				Type   string `json:"type"`
				Detail string `json:"detail"`
			}
			if err := json.Unmarshal(rpcErr.Data, &problem); err != nil {
				t.Fatalf("decode problem data: %v", err)
			}
			if problem.Type != protocol.ErrInvalidProtocolVersion.Error() {
				t.Errorf("problem type = %q, want %q", problem.Type, protocol.ErrInvalidProtocolVersion.Error())
			}
			if !strings.Contains(problem.Detail, protocol.Version) {
				t.Errorf("detail %q does not say which version this build serves", problem.Detail)
			}
		})
	}
}

func TestDispatchDoesNotMutateCallerRequestWhenStrippingMeta(t *testing.T) {
	req := &jsonrpc.Request{
		ID:     testID("1"),
		Method: "unknown.method",
		Params: jsontext.Value(fmt.Sprintf(`{"_meta":{"protocolVersion":%q},"value":1}`, protocol.Version)),
	}
	original := string(req.Params)
	newTestRouter(t, nil).Dispatch(context.Background(), req, delivery.Options{})
	if got := string(req.Params); got != original {
		t.Fatalf("Dispatch mutated caller params: got %s, want %s", got, original)
	}
}

// TestBindRequestMetaRefusesANonSuppressibleEvent checks that a client may suppress an
// ephemeral preview, never an authoritative event. Ignoring the illegal entry
// would leave the client believing it had opted out of a frame it will keep
// receiving — and a runtime that honored it would break the guarantee that
// discarding every ephemeral event still converges.
func TestDispatchRefusesANonSuppressibleEvent(t *testing.T) {
	req := &jsonrpc.Request{
		ID:     testID("1"),
		Method: "runs.cancel",
		Params: jsontext.Value(`{
			"_meta": {
				"clientCapabilities": { "excludedEphemeralEvents": ["item.completed"] }
			},
			"runId": "run_1"
		}`),
	}

	rpcErr := dispatchMetadataFailure(t, req)
	if rpcErr == nil {
		t.Fatal("Dispatch accepted an event outside the closed opt-out set")
	}
	if rpcErr.Code != codeInvalidParams {
		t.Fatalf("error code = %d, want invalid_params (%d)", rpcErr.Code, codeInvalidParams)
	}
}

func TestDispatchValidatesMetadataWireShape(t *testing.T) {
	for _, test := range []struct {
		name  string
		meta  string
		field string
	}{
		{
			name:  "unknown interrupt type",
			meta:  `{"clientCapabilities":{"interruptTypes":["telepathy"]}}`,
			field: "clientCapabilities.interruptTypes[0]",
		},
		{
			name:  "duplicate event exclusion",
			meta:  `{"clientCapabilities":{"excludedEphemeralEvents":["item.delta","item.delta"]}}`,
			field: "clientCapabilities.excludedEphemeralEvents",
		},
		{
			name:  "empty client identity",
			meta:  `{"clientInfo":{"name":"","version":"1.0.0"}}`,
			field: "clientInfo.name",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := &jsonrpc.Request{
				ID:     testID("1"),
				Method: "runs.cancel",
				Params: jsontext.Value(`{"_meta":` + test.meta + `,"runId":"run_1"}`),
			}

			rpcErr := dispatchMetadataFailure(t, req)
			if rpcErr == nil || rpcErr.Code != codeInvalidParams {
				t.Fatalf("error = %+v, want invalid_params", rpcErr)
			}
			var problem protocol.ProblemData
			if err := json.Unmarshal(rpcErr.Data, &problem); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			for _, field := range problem.Errors {
				if field.Field == test.field {
					return
				}
			}
			t.Fatalf("problem errors = %+v, want field %q", problem.Errors, test.field)
		})
	}
}

func TestExtractRequestMetaRejectsUnknownFields(t *testing.T) {
	req := &jsonrpc.Request{
		ID:     testID("1"),
		Method: "runs.cancel",
		Params: jsontext.Value(`{"_meta":{"capabilities":{}},"runId":"run_1"}`),
	}

	_, rpcErr := extractRequestMeta(req)
	if rpcErr == nil || rpcErr.Code != codeInvalidParams {
		t.Fatalf("error = %+v, want invalid_params", rpcErr)
	}
	var problem protocol.ProblemData
	if err := json.Unmarshal(rpcErr.Data, &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if !strings.Contains(problem.Detail, `unknown object member name "capabilities"`) {
		t.Fatalf("detail = %q, want unknown metadata field", problem.Detail)
	}
}

func dispatchMetadataFailure(t *testing.T, request *jsonrpc.Request) *jsonrpc.Error {
	t.Helper()
	result := newTestRouter(t, nil).Dispatch(t.Context(), request, delivery.Options{})
	if result.Response == nil {
		return nil
	}
	rpcError, _ := errors.AsType[*jsonrpc.Error](result.Response.Error)
	return rpcError
}

// The envelope is the sole author of request metadata, including an absent _meta.
func TestEnvelopeMetadataReplacesCallerAndContextMetadata(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprintf("explicit=%t", explicit), func(t *testing.T) {
			target := &metadataCaptureRuntime{capabilityRuntime: capabilityRuntime{}}
			router := newTestRouter(t, target)
			inherited := protocol.RequestMeta{ProtocolVersion: "invalid-parent", ClientInfo: &protocol.ClientInfo{Name: "parent"}}
			ctx := delivery.WithRequestMeta(t.Context(), inherited)
			supplied := protocol.RequestMeta{ProtocolVersion: "invalid-option", ClientInfo: &protocol.ClientInfo{Name: "option"}}
			raw := `{"sinceDays":7}`
			expected := protocol.RequestMeta{}
			if explicit {
				raw = fmt.Sprintf(`{"sinceDays":7,"_meta":{"protocolVersion":%q,"clientInfo":{"name":"envelope","version":"1"}}}`, protocol.Version)
				expected = protocol.RequestMeta{ProtocolVersion: protocol.Version, ClientInfo: &protocol.ClientInfo{Name: "envelope", Version: "1"}}
			}
			result := router.Dispatch(ctx, &jsonrpc.Request{ID: testID("metadata"), Method: "usage.summary", Params: jsontext.Value(raw)}, delivery.Options{RequestMeta: supplied})
			if result.Response == nil || result.Response.Error != nil {
				t.Fatalf("dispatch failed: %+v", result.Response)
			}
			if !target.called || !reflect.DeepEqual(target.metadata, expected) {
				t.Fatalf("operation metadata = %+v, want %+v", target.metadata, expected)
			}
			if parent, _ := delivery.RequestMetaFrom(ctx); !reflect.DeepEqual(parent, inherited) {
				t.Fatal("dispatch mutated parent metadata")
			}
		})
	}
}

type metadataCaptureRuntime struct {
	capabilityRuntime
	called   bool
	metadata protocol.RequestMeta
}

func (r *metadataCaptureRuntime) UsageSummary(ctx context.Context, request protocol.UsageSummaryRequest) (*protocol.UsageSummary, error) {
	r.called = true
	r.metadata, _ = delivery.RequestMetaFrom(ctx)
	return r.capabilityRuntime.UsageSummary(ctx, request)
}
