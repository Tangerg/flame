package plugin

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"strings"
	"testing"

	contractsdk "github.com/Tangerg/flame/runtime/capability"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type contractStatus string

func (contractStatus) JSONSchemaEnum() []string { return []string{"open", "resolved"} }

type contractInput struct {
	Status   contractStatus `json:"status"`
	Revision int            `json:"revision"`
}

func TestMethodBindsOneImmutableSchemaAcrossDescriptorAndWire(t *testing.T) {
	method, err := NewMethod[contractInput, contractInput](MethodConfig{Name: "reviews.update", Kind: contractsdk.Mutation, Target: "installation", Description: "Update a review."})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := method.Descriptor()
	descriptor.Input[0] = 'x'
	if method.Descriptor().Input[0] != '{' {
		t.Fatal("descriptor mutation changed method admission")
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil)
	backend := testBackend(t, server)
	calls := 0
	if err := method.RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, input Command[contractInput]) (contractInput, error) {
		calls++
		if input.ID != "original" {
			t.Fatal("logical identity changed")
		}
		return input.Input, nil
	}); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "caller"}, nil)
	if err := RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, client)
	input := contractInput{Status: "open", Revision: 1}
	if result, err := method.CallCommand(t.Context(), session, "original", input); err != nil || result != input {
		t.Fatalf("typed method = %+v, %v", result, err)
	}
	if _, err := method.Call(t.Context(), session, input); err == nil {
		t.Fatal("command accepted an unkeyed entrance")
	}
	input.Status = "unknown"
	if _, err := method.CallCommand(t.Context(), session, "original", input); err == nil {
		t.Fatal("input ignored generated enum")
	}
	if calls != 1 {
		t.Fatalf("rejected input executed business handler %d times", calls)
	}
	for _, raw := range []string{`{"status":"open"}`, `{"status":"open","revision":null}`, `{"status":"unknown","revision":1}`, `{"status":"open","revision":1,"extra":true}`} {
		var params Params[contractInput]
		data := []byte(`{"commandId":"original","input":` + raw + `}`)
		if err := json.Unmarshal(data, &params); err == nil {
			if err := method.contract.ValidateInput(params.inputJSON); err == nil {
				t.Fatalf("schema admitted original malformed input: %s", raw)
			}
		}
	}
}

func TestMethodOutputSchemaFailurePreservesUncertainty(t *testing.T) {
	method, err := NewMethod[struct{}, contractInput](MethodConfig{Name: "reviews.query", Kind: contractsdk.Query, Target: "installation", Description: "Inspect a review."})
	if err != nil {
		t.Fatal(err)
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil)
	backend := testBackend(t, server)
	if err := method.RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, Command[struct{}]) (contractInput, error) {
		return contractInput{Status: "unknown", Revision: 1}, nil
	}); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "caller"}, nil)
	if err := RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, client)
	if _, err := method.CallCommand(t.Context(), session, "command", struct{}{}); err == nil {
		t.Fatal("query accepted mutation identity")
	}
	_, err = method.Call(t.Context(), session, struct{}{})
	if err == nil {
		t.Fatal("malformed output became success")
	}
	if _, known := errors.AsType[*Failure](err); known {
		t.Fatalf("malformed output became known business refusal: %v", err)
	}
}

func TestMethodConstructionRefusesInvalidAndUnsupportedContracts(t *testing.T) {
	for _, config := range []MethodConfig{{}, {Name: "valid", Kind: "unknown", Target: "installation", Description: "Read."}, {Name: strings.Repeat("x", 257), Kind: contractsdk.Query, Target: "installation", Description: "Read."}} {
		if _, err := NewMethod[struct{}, struct{}](config); err == nil {
			t.Fatalf("invalid method admitted: %+v", config)
		}
	}
	if _, err := NewMethod[chan int, struct{}](MethodConfig{Name: "valid", Kind: contractsdk.Query, Target: "installation", Description: "Read."}); err == nil {
		t.Fatal("unsupported input shape admitted")
	}
	if _, err := NewMethod[any, struct{}](MethodConfig{Name: "valid", Kind: contractsdk.Query, Target: "installation", Description: "Read."}); err == nil {
		t.Fatal("nullable input schema conflicts with required non-null method envelope")
	}
}

type rawContractParams struct {
	sdk.ParamsBase
	Protocol  string         `json:"protocol"`
	Method    string         `json:"method"`
	Contract  string         `json:"contract"`
	Input     jsontext.Value `json:"input"`
	CommandID string         `json:"commandId"`
}

func TestMethodServerRejectsOriginalMissingMembersBeforeBusinessExecution(t *testing.T) {
	method, err := NewMethod[contractInput, contractInput](MethodConfig{Name: "reviews.update", Kind: contractsdk.Mutation, Target: "installation", Description: "Update."})
	if err != nil {
		t.Fatal(err)
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil)
	backend := testBackend(t, server)
	if err := method.RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, Command[contractInput]) (contractInput, error) {
		t.Error("malformed input reached business owner")
		return contractInput{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "caller"}, nil)
	if err := sdk.AddSendingCustomMethod[*rawContractParams, *Result[jsontext.Value]](client, invokeMethod); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, client)
	contract := method.contract.Identity()
	for _, raw := range []string{`{"status":"open"}`, `{"status":"open","revision":null}`, `{"status":"unknown","revision":1}`} {
		if _, err := sdk.CallCustomMethod[*rawContractParams, *Result[jsontext.Value]](t.Context(), session, invokeMethod, &rawContractParams{Protocol: contractsdk.Protocol, Method: method.Name(), Contract: contract.Digest(), Input: []byte(raw), CommandID: "original"}); err == nil {
			t.Fatalf("malformed input admitted: %s", raw)
		}
	}
}
