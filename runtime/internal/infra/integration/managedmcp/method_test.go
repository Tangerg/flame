package managedmcp

import (
	"context"
	"errors"
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
