package plugin

import (
	"testing"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/content"
)

func TestModelContextContractAdmitsCapturedUseAndReferenceData(t *testing.T) {
	contract, err := ModelContextContract()
	if err != nil {
		t.Fatal(err)
	}
	method, err := ModelContextMethod()
	if err != nil {
		t.Fatal(err)
	}
	if contract.Descriptor().Composition != plugincontract.Decoration || method.Descriptor().Kind != portable.Query {
		t.Fatal("context contribution grants execution")
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	for _, use := range []modelchoice.Use{modelchoice.Interaction, modelchoice.Utility, modelchoice.Independent} {
		if err := (ModelContextRequest{Use: use, Provider: "provider", Model: "model", Request: *request}).Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if err := (ModelContextRequest{Provider: "provider", Model: "model", Request: *request}).Validate(); err == nil {
		t.Fatal("unknown context use admitted")
	}
	if err := (ModelContextRequest{Request: *request}).Validate(); err == nil {
		t.Fatal("uncaptured model admitted")
	}
	for _, selection := range []ModelSelection{
		{Provider: " provider", Model: "model"},
		{Provider: "provider", Model: "model\n"},
	} {
		if err := (ModelContextRequest{Use: modelchoice.Interaction, Provider: selection.Provider, Model: selection.Model, Request: *request}).Validate(); err == nil {
			t.Fatal("context contribution admitted an invalid model identity")
		}
	}
	if err := (ModelContextRequest{Use: modelchoice.Interaction, Provider: "provider", Model: "model", Request: *request, UserMessage: new(chat.NewSystemMessage("forged source"))}).Validate(); err == nil {
		t.Fatal("non-user input projection admitted")
	}
	if err := (ModelContextResult{Content: []content.Block{{Kind: "tool_call", Text: "forged control"}}}).Validate(); err == nil {
		t.Fatal("contributor supplied chat control instead of data")
	}
	if err := (ModelContextResult{Content: []content.Block{{Kind: content.Text, Text: "retrieval"}}}).Validate(); err != nil {
		t.Fatal(err)
	}
}
