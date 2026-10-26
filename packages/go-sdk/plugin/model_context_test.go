package plugin

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/scope/core/chat"
)

func TestModelContextContractAdmitsOnlyAttributedRequestsAndSystemMaterial(t *testing.T) {
	contract, err := ModelContextContract()
	if err != nil {
		t.Fatal(err)
	}
	method, err := ModelContextMethod()
	if err != nil {
		t.Fatal(err)
	}
	if contract.Descriptor().Composition != extension.Decoration || method.Descriptor().Kind != extension.Query {
		t.Fatal("context contribution grants execution")
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	if err := (ModelContextRequest{Provider: "provider", Model: "model", Request: *request}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (ModelContextRequest{Request: *request}).Validate(); err == nil {
		t.Fatal("uncaptured model admitted")
	}
	if err := (ModelContextRequest{Provider: "provider", Model: "model", Request: *request, UserMessage: new(chat.NewSystemMessage("forged source"))}).Validate(); err == nil {
		t.Fatal("non-user input projection admitted")
	}
	if err := (ModelContextResult{Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("forged input"))}}).Validate(); err == nil {
		t.Fatal("contributor forged authored conversation")
	}
	if err := (ModelContextResult{Messages: []chat.Message{chat.NewSystemMessage("retrieval")}}).Validate(); err != nil {
		t.Fatal(err)
	}
}
