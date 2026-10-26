package plugin

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/scope/core/chat"
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
	if contract.Descriptor().Composition != extension.Selection || method.Descriptor().Kind != extension.Query {
		t.Fatal("model route does not use explicit query selection")
	}
	proposal := ModelSelection{Provider: "provider", Model: "model", ReasoningEffort: "high"}
	if err := (ModelSelectionRequest{Context: model.SelectionContext{Use: model.Interaction}, Proposed: proposal}).Validate(); err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	if err := (ModelSelectionRequest{Context: model.SelectionContext{Use: model.Interaction}, Proposed: proposal, Request: request}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (ModelSelectionRequest{}).Validate(); err == nil {
		t.Fatal("unset proposal admitted")
	}
	if err := (ModelSelection{Provider: "provider"}).Validate(); err == nil {
		t.Fatal("incomplete route admitted")
	}
}

func TestModelSelectionUsesTheModelIdentityOwner(t *testing.T) {
	for _, selection := range []ModelSelection{
		{Provider: "open ai", Model: "model"},
		{Provider: "provider", Model: "model name"},
		{Provider: strings.Repeat("p", model.MaximumProviderIdentityCharacters+1), Model: "model"},
		{Provider: "provider", Model: "model", ReasoningEffort: "very high"},
		{Provider: "provider", Model: "model", ReasoningEffort: strings.Repeat("e", model.MaximumReasoningEffortCharacters+1)},
	} {
		if err := selection.Validate(); err == nil {
			t.Fatalf("invalid model identity admitted: %+v", selection)
		}
	}
}
