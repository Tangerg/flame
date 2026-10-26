package plugin

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestBackendPublishesOneCurrentTransportWithIndependentExports(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil)
	backend := testBackend(t, server)
	calls := 0
	if err := testMethod[struct{}, string](t, "examples.read", extension.Query).RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, Command[struct{}]) (string, error) { return "read", nil }); err != nil {
		t.Fatal(err)
	}
	if err := testMethod[string, string](t, "examples.write", extension.Mutation).RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, Command[string]) (string, error) {
		calls++
		return "written", nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := testMethod[struct{}, string](t, "examples.read", extension.Query).RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, Command[struct{}]) (string, error) {
		return "replacement", nil
	}); err == nil {
		t.Fatal("duplicate export replaced its owner")
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "caller"}, nil)
	if err := RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, client)
	description, err := Describe(t.Context(), session)
	if err != nil || description.Protocol != extension.Protocol || len(description.Methods) != 2 {
		t.Fatalf("published description = %+v, %v", description, err)
	}
	if description.Methods[0].Name != "examples.read" || description.Methods[0].Kind != extension.Query || description.Methods[1].Kind != extension.Mutation {
		t.Fatalf("published semantics = %+v", description.Methods)
	}
	description.Methods[0].Input[0] = 'x'
	fresh, err := Describe(t.Context(), session)
	if err != nil || fresh.Methods[0].Input[0] != '{' {
		t.Fatal("client changed a published schema")
	}
	if err := testMethod[struct{}, string](t, "late", extension.Query).RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, Command[struct{}]) (string, error) { return "", nil }); err == nil {
		t.Fatal("published exports changed")
	}
	if _, err := testMethod[struct{}, string](t, "examples.write", extension.Query).Call(t.Context(), session, struct{}{}); err == nil {
		t.Fatal("caller relabeled mutation as a query")
	}
	if _, err := testMethod[struct{}, string](t, "examples.read", extension.Mutation).CallCommand(t.Context(), session, "command", struct{}{}); err == nil {
		t.Fatal("query accepted mutation identity")
	}
	if _, err := testMethod[struct{}, string](t, "missing", extension.Query).Call(t.Context(), session, struct{}{}); err == nil {
		t.Fatal("missing export fell back to another method")
	}
	if result, err := testMethod[string, string](t, "examples.write", extension.Mutation).CallCommand(t.Context(), session, "command", "input"); err != nil || result != "written" || calls != 1 {
		t.Fatalf("one generic command = %s, %d, %v", result, calls, err)
	}
	if err := sdk.AddSendingCustomMethod[*methodParams[struct{}], *methodResult[jsontext.Value]](client, "examples.read"); err != nil {
		t.Fatal(err)
	}
	if _, err := sdk.CallCustomMethod[*methodParams[struct{}], *methodResult[jsontext.Value]](t.Context(), session, "examples.read", &methodParams[struct{}]{}); err == nil {
		t.Fatal("former named RPC was retained")
	}
}

func TestBackendRejectsOldOrMalformedTransportBeforeInvocation(t *testing.T) {
	for _, data := range []string{
		`{}`, `{"protocol":"old","method":"read","input":{}}`,
		`{"protocol":"` + extension.Protocol + `","method":"read"}`,
		`{"protocol":"` + extension.Protocol + `","method":"read","input":{}}`,
		`{"protocol":"` + extension.Protocol + `","method":"read","input":{},"extra":true}`,
	} {
		var request invokeParams
		if err := json.Unmarshal([]byte(data), &request); err == nil {
			t.Fatalf("malformed invocation admitted: %s", data)
		}
	}
	for _, data := range []string{`{}`, `{"protocol":"old"}`, `{"protocol":"` + extension.Protocol + `","extra":true}`} {
		var request protocolParams
		if err := json.Unmarshal([]byte(data), &request); err == nil {
			t.Fatalf("malformed handshake admitted: %s", data)
		}
	}
	for _, extra := range []string{`,"export":"reader"`, `,"capability":"` + strings.Repeat("a", 64) + `"`, `,"export":"../reader","capability":"` + strings.Repeat("a", 64) + `"`} {
		data := `{"protocol":"` + extension.Protocol + `","method":"read","contract":"` + strings.Repeat("a", 64) + `","input":{}` + extra + `}`
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
