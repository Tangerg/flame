package modelprovider

import (
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"
	framevalue "github.com/Tangerg/frame/value"
	coreinvocation "github.com/Tangerg/scope/core/invocation"

	"encoding/json/jsontext"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/catalog"
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
	request := ChatRequest{Handle: Handle{ID: HandleID(strings.Repeat("a", 64))}, Request: chat.Request{
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("question"))},
		Options:  chat.Options{Model: "exact-model", MaxOutputTokens: new(int64(64))},
	}}
	encoded, err := framevalue.EncodeJSON(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := method.ValidateInput(encoded); err != nil {
		t.Fatal(err)
	}
	incomplete, err := framevalue.ParseJSON([]byte(`{"handle":{"id":"` + request.Handle.ID + `"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := method.ValidateInput(incomplete); err == nil {
		t.Fatal("missing model request was admitted")
	}
}

func TestEmbeddingEnvelopesUseNativeAdmissionWithoutLosingUsage(t *testing.T) {
	request := EmbeddingRequest{Handle: Handle{ID: HandleID(strings.Repeat("a", 64))}, Request: jsontext.Value(`{"texts":["question"],"options":{"model":"embedding-model","dimensions":2}}`)}
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
	for _, endpoint := range []string{"relative/path", "file:///tmp/secret", "https://user:password@host/", "https://host/#fragment", "https://:443/"} {
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
	failure := &Failure{Kind: ProviderUnavailable, Execution: coreinvocation.FailureUnknown}
	if err := (ChatResult{Failure: failure}).Validate(); err != nil {
		t.Fatal(err)
	}
	failure.Execution = "invalid"
	if err := (ChatResult{Failure: failure}).Validate(); err == nil {
		t.Fatal("invented execution evidence admitted")
	}
	if err := (TokenCount{Count: new(int64(1)), Failure: &Failure{Kind: Internal, Execution: coreinvocation.FailureUnknown}}).Validate(); err == nil {
		t.Fatal("conflicting count outcomes admitted")
	}
}

func TestConstructionDeclaresItsExactIdentityWhenCatalogFactsAreUnknown(t *testing.T) {
	client := Chat{Handle: Handle{ID: HandleID(strings.Repeat("a", 64))}, Original: "original-open", Provider: "custom-provider", Model: "private-model"}
	if err := client.Validate(); err != nil {
		t.Fatal(err)
	}
	client.Metadata = &catalog.Model{ID: "another-model"}
	if err := client.Validate(); err == nil {
		t.Fatal("metadata replaced the captured model")
	}
	client.Metadata = nil
	client.Provider = ""
	if err := client.Validate(); err == nil {
		t.Fatal("unknown catalog facts hid a missing provider identity")
	}
}

func TestCheckpointIdentityIsIndependentOfExecutableHandles(t *testing.T) {
	id, err := ContractIdentity()
	if err != nil {
		t.Fatal(err)
	}
	c, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	if c.Identity() != id {
		t.Fatal("checkpoint identity differs from executable contract")
	}
	method, ok := c.Method(c.Descriptor().Methods[0].Name)
	if !ok {
		t.Fatal("missing declared method")
	}
	*method = portable.Method{}
	*c = plugincontract.Capability{}
	id.Bytes()[0] = 0
	after, err := ContractIdentity()
	if err != nil || after != id || after.Bytes()[0] == 0 {
		t.Fatal("caller changed published contract identity", err)
	}
}

func TestProviderMethodsKeepHandlesAndSchemasIsolated(t *testing.T) {
	verifyMethodHandle(t, ProfilesMethod)
	verifyMethodHandle(t, RemoteModelsMethod)
	verifyMethodHandle(t, VerifyMethod)
	verifyMethodHandle(t, OpenChatMethod)
	verifyMethodHandle(t, OpenEmbeddingMethod)
	verifyMethodHandle(t, ReopenChatMethod)
	verifyMethodHandle(t, CloseMethod)
	verifyMethodHandle(t, CallMethod)
	verifyMethodHandle(t, StreamMethod)
	verifyMethodHandle(t, CountMethod)
	verifyMethodHandle(t, EmbedMethod)
}

func verifyMethodHandle[T any, P interface {
	*T
	Descriptor() portable.MethodDescriptor
}](t *testing.T, load func() (P, error)) {
	t.Helper()
	method, err := load()
	if err != nil {
		t.Fatal(err)
	}
	before := method.Descriptor()
	exposed := method.Descriptor()
	exposed.Input[0] = 0
	exposed.Output[0] = 0
	if len(exposed.Stream) > 0 {
		exposed.Stream[0] = 0
	}
	var zero T
	*method = zero
	next, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if method == next || !reflect.DeepEqual(next.Descriptor(), before) {
		t.Fatalf("%s re-exposed mutated method state", before.Name)
	}
}
