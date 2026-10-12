package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

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
			result *Result[string]
		}{
			{"neither", &Result[string]{}},
			{"both", &Result[string]{Value: &value, Failure: &Failure{Code: "refused", Message: "failed"}}},
		} {
			t.Run(caller.name+"/"+reply.name, func(t *testing.T) {
				session := replySession(t, reply.result)
				var err error
				if caller.command {
					_, err = CallCommand[string, string](t.Context(), session, "flame.test/reply.v1", "command", "input")
				} else {
					_, err = Call[string, string](t.Context(), session, "flame.test/reply.v1", "input")
				}
				if err == nil {
					t.Fatal("malformed backend reply succeeded")
				}
				if _, classified := errors.AsType[*Failure](err); classified {
					t.Fatalf("malformed backend reply became a classified failure: %v", err)
				}
			})
		}
	}
}

func TestMethodsPreserveKnownRefusalsAndUnknownFailures(t *testing.T) {
	refused, storage := errors.New("revision conflict"), errors.New("commit result unknown")
	for _, command := range []bool{false, true} {
		for _, cause := range []error{refused, storage} {
			t.Run(cause.Error()+"/"+map[bool]string{false: "method", true: "command"}[command], func(t *testing.T) {
				session := methodSession(t, func(server *sdk.Server) error {
					classify := func(err error) string {
						if errors.Is(err, refused) {
							return "revisionConflict"
						}
						return ""
					}
					if command {
						return RegisterCommand(server, "flame.test/failure", classify, func(context.Context, *sdk.ServerSession, Command[string]) (string, error) { return "", cause })
					}
					return RegisterServer(server, "flame.test/failure", classify, func(context.Context, *sdk.ServerSession, string) (string, error) { return "", cause })
				}, "flame.test/failure")
				var err error
				if command {
					_, err = CallCommand[string, string](t.Context(), session, "flame.test/failure", "logical-command", "input")
				} else {
					_, err = Call[string, string](t.Context(), session, "flame.test/failure", "input")
				}
				if err == nil {
					t.Fatal("failed operation succeeded")
				}
				failure, known := errors.AsType[*Failure](err)
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
	Input     any    `json:"input,omitzero"`
	CommandID string `json:"commandId,omitzero"`
	Extra     bool   `json:"extra,omitzero"`
}

func TestCommandsRejectMalformedEnvelopeBeforeMutation(t *testing.T) {
	var calls atomic.Int64
	server := sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil)
	if err := RegisterCommand(server, "flame.test/admission", nil, func(_ context.Context, _ *sdk.ServerSession, command Command[string]) (string, error) {
		calls.Add(1)
		return command.Input, nil
	}); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "caller"}, nil)
	if err := sdk.AddSendingCustomMethod[*malformedParams, *Result[string]](client, "flame.test/admission"); err != nil {
		t.Fatal(err)
	}
	session := connectMethodSessions(t, server, client)
	for _, params := range []malformedParams{
		{Input: "input"},
		{CommandID: "command"},
		{Input: (*string)(nil), CommandID: "command"},
		{Input: "input", CommandID: strings.Repeat("c", 257)},
		{Input: "input", CommandID: "command", Extra: true},
	} {
		if result, err := sdk.CallCustomMethod[*malformedParams, *Result[string]](t.Context(), session, "flame.test/admission", &params); err == nil || result != nil {
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
		var params Params[input]
		if err := json.Unmarshal([]byte(wire), &params); err == nil {
			t.Fatalf("unknown parameter member accepted: %s", wire)
		}
	}
	for _, wire := range []string{
		`{"value":{"text":"a","extra":true}}`,
		`{"value":{"text":"a"},"extra":true}`,
		`{"failure":{"code":"conflict","message":"failed","extra":true}}`,
	} {
		var result Result[input]
		if err := json.Unmarshal([]byte(wire), &result); err == nil {
			t.Fatalf("unknown result member accepted: %s", wire)
		}
	}
	var params Params[input]
	if err := json.Unmarshal([]byte(`{"_meta":{"caller":"retained"},"input":{"text":"a"},"commandId":"command"}`), &params); err != nil || params.GetMeta()["caller"] != "retained" || params.Input.Text != "a" || params.CommandID != "command" {
		t.Fatalf("admitted parameters lost identity, input or MCP metadata: %+v, %v", params, err)
	}
}

func TestMethodReplyPreservesExplicitNullableValue(t *testing.T) {
	for _, value := range []any{struct{}{}, 0, false, "", []string{}} {
		reply, err := Reply(value, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(reply)
		if err != nil {
			t.Fatal(err)
		}
		var decoded Result[any]
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if _, err := decoded.value("explicit empty outcome"); err != nil {
			t.Fatalf("empty/zero value was omitted: %s, %v", encoded, err)
		}
	}
	var nullable Result[[]string]
	if err := json.Unmarshal([]byte(`{"value":null}`), &nullable); err != nil {
		t.Fatal(err)
	}
	if value, err := nullable.value("empty query"); err != nil || value != nil {
		t.Fatalf("explicit nullable value became an unconfirmed reply: %v, %v", value, err)
	}
	var scalar Result[string]
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

func methodSession(t *testing.T, register func(*sdk.Server) error, name string) *sdk.ClientSession {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil)
	if err := register(server); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "caller"}, nil)
	if err := RegisterClient[string, string](client, name); err != nil {
		t.Fatal(err)
	}
	return connectMethodSessions(t, server, client)
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
	session := replySession(t, &Result[string]{Failure: &Failure{Code: "refused", Message: "failed"}})
	_, err := CallCommand[string, string](t.Context(), session, "flame.test/reply.v1", "command", "input")
	failure, ok := errors.AsType[*Failure](err)
	if !ok || failure.Code != "refused" || failure.Message != "failed" || err.Error() != "failed" {
		t.Fatalf("backend classification lost: %v", err)
	}
}

func replySession(t *testing.T, result *Result[string]) *sdk.ClientSession {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: "backend"}, nil)
	if err := sdk.AddReceivingCustomMethod(server, "flame.test/reply.v1", func(context.Context, *sdk.ServerSession, *Params[string]) (*Result[string], error) {
		return result, nil
	}); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "host"}, nil)
	if err := RegisterClient[string, string](client, "flame.test/reply.v1"); err != nil {
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
