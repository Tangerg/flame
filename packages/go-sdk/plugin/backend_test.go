package plugin

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"
	"testing"

	contractsdk "github.com/Tangerg/flame/packages/go-sdk/capability"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestBackendPublishesOneCurrentTransportWithIndependentExports(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil)
	backend := testBackend(t, server)
	calls := 0
	if err := RegisterServer(backend, "examples.read", nil, func(context.Context, *sdk.ServerSession, struct{}) (string, error) { return "read", nil }); err != nil {
		t.Fatal(err)
	}
	if err := RegisterCommand(backend, "examples.write", nil, func(context.Context, *sdk.ServerSession, Command[string]) (string, error) {
		calls++
		return "written", nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := RegisterServer(backend, "examples.read", nil, func(context.Context, *sdk.ServerSession, struct{}) (string, error) { return "replacement", nil }); err == nil {
		t.Fatal("duplicate export replaced its owner")
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "caller"}, nil)
	if err := RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, client)
	description, err := Describe(t.Context(), session)
	if err != nil || description.Protocol != contractsdk.Protocol || len(description.Methods) != 2 {
		t.Fatalf("published description = %+v, %v", description, err)
	}
	if description.Methods[0].Name != "examples.read" || description.Methods[0].Kind != contractsdk.Query || description.Methods[1].Kind != contractsdk.Mutation {
		t.Fatalf("published semantics = %+v", description.Methods)
	}
	description.Methods[0].Input[0] = 'x'
	fresh, err := Describe(t.Context(), session)
	if err != nil || fresh.Methods[0].Input[0] != '{' {
		t.Fatal("client changed a published schema")
	}
	if err := RegisterServer(backend, "late", nil, func(context.Context, *sdk.ServerSession, struct{}) (string, error) { return "", nil }); err == nil {
		t.Fatal("published exports changed")
	}
	if _, err := Call[struct{}, string](t.Context(), session, "examples.write", struct{}{}); err == nil {
		t.Fatal("caller relabeled mutation as a query")
	}
	if _, err := CallCommand[struct{}, string](t.Context(), session, "examples.read", "command", struct{}{}); err == nil {
		t.Fatal("query accepted mutation identity")
	}
	if _, err := Call[struct{}, string](t.Context(), session, "missing", struct{}{}); err == nil {
		t.Fatal("missing export fell back to another method")
	}
	if result, err := CallCommand[string, string](t.Context(), session, "examples.write", "command", "input"); err != nil || result != "written" || calls != 1 {
		t.Fatalf("one generic command = %s, %d, %v", result, calls, err)
	}
	if err := sdk.AddSendingCustomMethod[*Params[struct{}], *Result[jsontext.Value]](client, "examples.read"); err != nil {
		t.Fatal(err)
	}
	if _, err := sdk.CallCustomMethod[*Params[struct{}], *Result[jsontext.Value]](t.Context(), session, "examples.read", &Params[struct{}]{}); err == nil {
		t.Fatal("former named RPC was retained")
	}
}

func TestBackendRejectsOldOrMalformedTransportBeforeInvocation(t *testing.T) {
	for _, data := range []string{
		`{}`, `{"protocol":"old","method":"read","input":{}}`,
		`{"protocol":"` + contractsdk.Protocol + `","method":"read"}`,
		`{"protocol":"` + contractsdk.Protocol + `","method":"read","input":{}}`,
		`{"protocol":"` + contractsdk.Protocol + `","method":"read","input":{},"extra":true}`,
	} {
		var request invokeParams
		if err := json.Unmarshal([]byte(data), &request); err == nil {
			t.Fatalf("malformed invocation admitted: %s", data)
		}
	}
	for _, data := range []string{`{}`, `{"protocol":"old"}`, `{"protocol":"` + contractsdk.Protocol + `","extra":true}`} {
		var request protocolParams
		if err := json.Unmarshal([]byte(data), &request); err == nil {
			t.Fatalf("malformed handshake admitted: %s", data)
		}
	}
	for _, extra := range []string{`,"export":"reader"`, `,"capability":"` + strings.Repeat("a", 64) + `"`, `,"export":"../reader","capability":"` + strings.Repeat("a", 64) + `"`} {
		data := `{"protocol":"` + contractsdk.Protocol + `","method":"read","contract":"` + strings.Repeat("a", 64) + `","input":{}` + extra + `}`
		var request invokeParams
		if err := json.Unmarshal([]byte(data), &request); err == nil {
			t.Fatalf("incomplete capability binding admitted: %s", data)
		}
	}
	if _, err := NewBackend(nil); err == nil {
		t.Fatal("backend has no server")
	}
	backend := testBackend(t, sdk.NewServer(&sdk.Implementation{Name: "empty"}, nil))
	if err := backend.Publish(); err == nil {
		t.Fatal("empty backend published")
	}
}
