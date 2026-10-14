package plugin

import (
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/scope/core/chat"
	"testing"
)

func TestModelSelectionContractRequiresExactProposalAndPreservesInputAbsence(t *testing.T) {
	contract, err := ModelSelectionContract()
	if err != nil {
		t.Fatal(err)
	}
	method, err := ModelSelectionMethod()
	if err != nil {
		t.Fatal(err)
	}
	if contract.Descriptor().Composition != capability.Selection || method.Descriptor().Kind != capability.Query {
		t.Fatal("model route does not use explicit query selection")
	}
	proposal := ModelSelection{Provider: "provider", Model: "model", ReasoningEffort: "high"}
	if err := (ModelSelectionRequest{Proposed: proposal}).Validate(); err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	if err := (ModelSelectionRequest{Proposed: proposal, Request: request}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (ModelSelectionRequest{}).Validate(); err == nil {
		t.Fatal("unset proposal admitted")
	}
	if err := (ModelSelection{Provider: "provider"}).Validate(); err == nil {
		t.Fatal("incomplete route admitted")
	}
}
