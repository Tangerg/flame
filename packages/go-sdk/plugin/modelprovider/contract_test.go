package modelprovider

import (
	"encoding/json/jsontext"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/scope/core/chat"
)

func TestContractAdmitsTheCompleteNativeRequestAndRefusesMissingInput(t *testing.T) {
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	method, ok := contract.Method("flame.model-provider/call.v1")
	if !ok {
		t.Fatal("missing model call")
	}
	request := ChatRequest{Handle: Handle{ID: strings.Repeat("a", 64)}, Request: chat.Request{
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("question"))},
		Options:  chat.Options{Model: "exact-model", MaxOutputTokens: new(int64(64))},
	}}
	encoded, err := jsonvalue.Encode(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := method.ValidateInput(encoded); err != nil {
		t.Fatal(err)
	}
	incomplete, err := jsonvalue.Parse([]byte(`{"handle":{"id":"` + request.Handle.ID + `"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := method.ValidateInput(incomplete); err == nil {
		t.Fatal("missing model request was admitted")
	}
}

func TestEmbeddingEnvelopesUseNativeAdmissionWithoutLosingUsage(t *testing.T) {
	request := EmbeddingRequest{Handle: Handle{ID: strings.Repeat("a", 64)}, Request: jsontext.Value(`{"texts":["question"],"options":{"model":"embedding-model","dimensions":2}}`)}
	native, err := request.Native()
	if err != nil || len(native.Texts) != 1 || *native.Options.Dimensions != 2 {
		t.Fatalf("complete embedding input = %+v %v", native, err)
	}
	result := EmbeddingResult{Response: jsontext.Value(`{"outputs":[{"embedding":[0.25,0.75]}],"metadata":{"model":"served-model","usage":{"input_tokens":13}}}`)}
	response, err := result.Native()
	if err != nil || response.Metadata.Usage.InputTokens != 13 || response.Metadata.Model != "served-model" {
		t.Fatalf("complete embedding response = %+v %v", response, err)
	}
	result.Response = jsontext.Value(`{"outputs":[{"embedding":[null,0.75]}]}`)
	if err := result.Validate(); err == nil {
		t.Fatal("a null coordinate became a numeric vector")
	}
	request.Request = jsontext.Value(`{"texts":["question"],"options":{"model":"embedding-model"},"unknown":true}`)
	if _, err := request.Native(); err == nil {
		t.Fatal("unknown native embedding input was admitted")
	}
}

func TestConnectionFormattingNeverExposesCredentialsOrEndpointSecrets(t *testing.T) {
	connection := Connection{Provider: "custom", APIKey: "private-key", BaseURL: "https://endpoint.test/?token=private-token"}
	if err := connection.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
		formatted := fmt.Sprintf(format, connection)
		if strings.Contains(formatted, "private-") {
			t.Fatalf("connection leaked through %s", format)
		}
	}
	for _, endpoint := range []string{"relative/path", "file:///tmp/secret", "https://user:password@host/", "https://host/#fragment"} {
		connection.BaseURL = endpoint
		if err := connection.Validate(); err == nil {
			t.Fatalf("invalid endpoint admitted: %s", endpoint)
		}
	}
}

func TestFailureEvidenceCannotTurnAnUnknownExchangeIntoSuccess(t *testing.T) {
	if err := (ChatResult{}).Validate(); err == nil {
		t.Fatal("missing outcome became success")
	}
	failure := &Failure{Kind: ProviderUnavailable, Execution: chat.CallFailureUnknown}
	if err := (ChatResult{Failure: failure}).Validate(); err != nil {
		t.Fatal(err)
	}
	failure.Execution = "invalid"
	if err := (ChatResult{Failure: failure}).Validate(); err == nil {
		t.Fatal("invented execution evidence admitted")
	}
	if err := (TokenCount{Count: new(int64(1)), Failure: &Failure{Kind: Internal, Execution: chat.CallFailureUnknown}}).Validate(); err == nil {
		t.Fatal("conflicting count outcomes admitted")
	}
}
