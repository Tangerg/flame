package mcp

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestCallsRejectAmbiguousBackendReplies(t *testing.T) {
	value := "success"
	for _, caller := range []struct {
		name    string
		command bool
	}{{"read", false}, {"command", true}} {
		for _, reply := range []struct {
			name   string
			result *methodResult[string]
		}{
			{"neither", &methodResult[string]{}},
			{"both", &methodResult[string]{Value: &value, Failure: &portable.Refusal{Code: "refused", Message: "failed"}}},
		} {
			t.Run(caller.name+"/"+reply.name, func(t *testing.T) {
				session := replySession(t, reply.result)
				var err error
				if caller.command {
					_, err = testMethod[string, string](t, "flame.test/reply.v1", portable.Mutation).CallCommand(t.Context(), session, "command", "input")
				} else {
					_, err = testMethod[string, string](t, "flame.test/reply.v1", portable.Query).Call(t.Context(), session, "input")
				}
				if err == nil {
					t.Fatal("malformed backend reply succeeded")
				}
				if _, classified := errors.AsType[*portable.Refusal](err); classified {
					t.Fatalf("malformed backend reply became a classified failure: %v", err)
				}
			})
		}
	}
}

func TestMethodsPreserveKnownRefusalsAndUnknownFailures(t *testing.T) {
	refused, storage := errors.New("revision conflict"), errors.New("commit result unknown")
	for _, operation := range []struct {
		name string
		kind portable.MethodKind
	}{{"query", portable.Query}, {"command", portable.Mutation}} {
		for _, cause := range []error{refused, storage} {
			t.Run(cause.Error()+"/"+operation.name, func(t *testing.T) {
				session := methodSession(t, func(server *Backend) error {
					classify := func(err error) string {
						if errors.Is(err, refused) {
							return "revisionConflict"
						}
						return ""
					}
					return testMethod[string, string](t, "flame.test/failure", operation.kind).RegisterServer(server, classify, func(context.Context, *sdk.ServerSession, Command[string]) (string, error) { return "", cause })
				}, "flame.test/failure")
				var err error
				if operation.kind == portable.Mutation {
					_, err = testMethod[string, string](t, "flame.test/failure", portable.Mutation).CallCommand(t.Context(), session, "logical-command", "input")
				} else {
					_, err = testMethod[string, string](t, "flame.test/failure", portable.Query).Call(t.Context(), session, "input")
				}
				if err == nil {
					t.Fatal("failed operation succeeded")
				}
				failure, known := errors.AsType[*portable.Refusal](err)
				if known != (cause == refused) {
					t.Fatalf("known business classification = %v; cause = %v", err, cause)
				}
				if known && failure.Code != "revisionConflict" {
					t.Fatalf("business category changed: %+v", failure)
				}
			})
		}
	}
}

type malformedParams struct {
	sdk.ParamsBase
	Protocol  string `json:"protocol"`
	Method    string `json:"method"`
	Contract  string `json:"contract"`
	Input     any    `json:"input,omitzero"`
	CommandID string `json:"commandId,omitzero"`
	Extra     bool   `json:"extra,omitzero"`
}

func TestCommandsRejectMalformedEnvelopeBeforeMutation(t *testing.T) {
	var calls atomic.Int64
	server := sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil)
	backend := testBackend(t, server)
	if err := testMethod[string, string](t, "flame.test/admission", portable.Mutation).RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command Command[string]) (string, error) {
		calls.Add(1)
		return command.Input, nil
	}); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "caller"}, nil)
	if err := sdk.AddSendingCustomMethod[*malformedParams, *methodResult[jsontext.Value]](client, InvokeMethod); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, client)
	method, err := NewMethod[string, string](MethodConfig{Name: "flame.test/admission", Kind: portable.Mutation, Target: "installation", Description: "admission"})
	if err != nil {
		t.Fatal(err)
	}
	contract := method.contract.Identity()
	for _, params := range []malformedParams{
		{Input: "input"},
		{CommandID: "command"},
		{Input: (*string)(nil), CommandID: "command"},
		{Input: "input", CommandID: strings.Repeat("c", MaxCommandIDBytes+1)},
		{Input: "input", CommandID: "command", Extra: true},
	} {
		params.Protocol = portable.Protocol
		params.Method = "flame.test/admission"
		params.Contract = contract.Digest()
		if result, err := sdk.CallCustomMethod[*malformedParams, *methodResult[jsontext.Value]](t.Context(), session, InvokeMethod, &params); err == nil || result != nil {
			t.Fatalf("malformed command reached a result: %+v, %v", result, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("malformed admission ran %d mutations", calls.Load())
	}
}

func TestMethodWireRejectsUnknownInputAndOutputFields(t *testing.T) {
	type input struct {
		Text string `json:"text"`
	}
	for _, wire := range []string{
		`{"input":{"text":"a","extra":true}}`,
		`{"input":{"text":"a"},"extra":true}`,
	} {
		var params methodParams[input]
		if err := json.Unmarshal([]byte(wire), &params); err == nil {
			t.Fatalf("unknown parameter member accepted: %s", wire)
		}
	}
	for _, wire := range []string{
		`{"value":{"text":"a","extra":true}}`,
		`{"value":{"text":"a"},"extra":true}`,
		`{"failure":{"code":"conflict","message":"failed","extra":true}}`,
	} {
		var result methodResult[input]
		if err := json.Unmarshal([]byte(wire), &result); err == nil {
			t.Fatalf("unknown result member accepted: %s", wire)
		}
	}
	var params methodParams[input]
	if err := json.Unmarshal([]byte(`{"_meta":{"caller":"retained"},"input":{"text":"a"},"commandId":"command"}`), &params); err != nil || string(params.GetMeta()["caller"]) != `"retained"` || params.Input.Text != "a" || params.CommandID != "command" {
		t.Fatalf("admitted parameters lost identity, input or MCP metadata: %+v, %v", params, err)
	}
}

func TestMethodReplyPreservesExplicitNullableValue(t *testing.T) {
	for _, value := range []any{struct{}{}, 0, false, "", []string{}} {
		reply, err := reply(value, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(reply)
		if err != nil {
			t.Fatal(err)
		}
		var decoded methodResult[any]
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if _, err := decoded.value("explicit empty outcome"); err != nil {
			t.Fatalf("empty/zero value was omitted: %s, %v", encoded, err)
		}
	}
	var nullable methodResult[[]string]
	if err := json.Unmarshal([]byte(`{"value":null}`), &nullable); err != nil {
		t.Fatal(err)
	}
	if value, err := nullable.value("empty query"); err != nil || value != nil {
		t.Fatalf("explicit nullable value became an unconfirmed reply: %v, %v", value, err)
	}
	var scalar methodResult[string]
	if err := json.Unmarshal([]byte(`{"value":null}`), &scalar); err == nil {
		t.Fatal("non-nullable result accepted null")
	}
	if err := json.Unmarshal([]byte(`{}`), &nullable); err != nil {
		t.Fatal(err)
	}
	if _, err := nullable.value("missing query reply"); err == nil {
		t.Fatal("missing result became a successful empty query")
	}
}

func methodSession(t *testing.T, register func(*Backend) error, name string) *sdk.ClientSession {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil)
	backend := testBackend(t, server)
	if err := register(backend); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "caller"}, nil)
	if err := RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	return connectPublishedBackend(t, backend, client)
}

func connectMethodSessions(t *testing.T, server *sdk.Server, client *sdk.Client) *sdk.ClientSession {
	t.Helper()
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
	return session
}

func TestCallsPreserveBackendFailureClassification(t *testing.T) {
	session := replySession(t, &methodResult[string]{Failure: &portable.Refusal{Code: "refused", Message: "failed"}})
	_, err := testMethod[string, string](t, "flame.test/reply.v1", portable.Mutation).CallCommand(t.Context(), session, "command", "input")
	failure, ok := errors.AsType[*portable.Refusal](err)
	if !ok || failure.Code != "refused" || failure.Message != "failed" || err.Error() != "failed" {
		t.Fatalf("backend classification lost: %v", err)
	}
}

func replySession(t *testing.T, result *methodResult[string]) *sdk.ClientSession {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "backend"}, nil)
	method, err := NewMethod[string, string](MethodConfig{Name: "flame.test/reply.v1", Kind: portable.Mutation, Target: "installation", Description: "reply"})
	if err != nil {
		t.Fatal(err)
	}
	if err := sdk.AddReceivingCustomMethod(server, DescribeMethod, func(context.Context, *sdk.ServerSession, *protocolParams) (*methodResult[Description], error) {
		return reply(Description{Protocol: portable.Protocol, Methods: []portable.MethodDescriptor{method.Descriptor()}}, nil, nil)
	}); err != nil {
		t.Fatal(err)
	}
	if err := sdk.AddReceivingCustomMethod(server, InvokeMethod, func(context.Context, *sdk.ServerSession, *invokeParams) (*methodResult[string], error) {
		return result, nil
	}); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "host"}, nil)
	if err := RegisterClient(client); err != nil {
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
	return session
}

func testBackend(t *testing.T, server *sdk.Server) *Backend {
	t.Helper()
	backend, err := NewBackend(server)
	if err != nil {
		t.Fatal(err)
	}
	return backend
}
func connectPublishedBackend(t *testing.T, backend *Backend, client *sdk.Client) *sdk.ClientSession {
	t.Helper()
	if err := backend.Publish(); err != nil {
		t.Fatal(err)
	}
	return connectMethodSessions(t, backend.server, client)
}

func testMethod[P, R any](t *testing.T, name string, kind portable.MethodKind) *Method[P, R] {
	t.Helper()
	method, err := NewMethod[P, R](MethodConfig{Name: name, Kind: kind, Target: "installation", Description: name})
	if err != nil {
		t.Fatal(err)
	}
	return method
}
