package modelref

import (
	"testing"
	"time"

	"github.com/Tangerg/scope/core/chat"
)

func TestEndpointInvocationRejectsPointerRequestVariants(t *testing.T) {
	record := endpointRecord(t)
	request := record.Invocation.Request.(ChatEndpointRequest)
	for _, test := range []struct {
		name    string
		request EndpointRequest
	}{
		{"mutable_chat", &request},
		{"nil_chat", (*ChatEndpointRequest)(nil)},
		{"nil_embedding", (*EmbeddingEndpointRequest)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			invocation := record.Invocation
			invocation.Request = test.request
			if err := invocation.Validate(); err == nil {
				t.Fatal("invocation accepted a pointer request variant")
			}
			snapshot := record
			snapshot.Invocation = invocation
			if _, err := RestoreEndpointObservation(snapshot); err == nil {
				t.Fatal("observation restoration accepted a pointer request variant")
			}
		})
	}
}

func TestEndpointOutcomeRejectsPointerResponseVariants(t *testing.T) {
	invocation := endpointRecord(t).Invocation
	message := chat.NewAssistantMessage(chat.NewTextPart("captured response"))
	response := ChatEndpointResponse{Chat: &chat.Response{
		Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop},
	}}
	for _, test := range []struct {
		name     string
		response EndpointResponse
	}{
		{"mutable_chat", &response},
		{"nil_chat", (*ChatEndpointResponse)(nil)},
		{"nil_embedding", (*EmbeddingEndpointResponse)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			outcome := EndpointOutcome{
				Response: test.response, FinishedAt: invocation.StartedAt.Add(time.Second),
			}
			if _, err := outcome.State(invocation); err == nil {
				t.Fatal("settlement accepted a pointer response variant")
			}
		})
	}
}
