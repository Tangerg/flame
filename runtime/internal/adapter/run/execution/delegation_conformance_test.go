package execution

import (
	"context"
	"testing"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	"github.com/Tangerg/scope/agent/run/runtest"
	"github.com/Tangerg/scope/core/chat"
)

func TestDelegatedDefinitionConformance(t *testing.T) {
	runtime, err := scoperun.New(t.Context(), scoperun.Config{Store: scoperun.NewMemoryStore(), Build: "delegation-conformance"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	inner, err := interaction.Compile(runtime.Compiler(), interaction.Config{
		Name: "conformance.interaction", Description: "Delegated Interaction conformance.", Revision: "delegation-conformance",
		Model: interaction.CallModel(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
			t.Error("Definition conformance dispatched external work")
			return nil, context.Canceled
		})),
	})
	if err != nil {
		t.Fatal(err)
	}
	program, err := compileDelegatedInteraction(
		runtime.Compiler(), "conformance.delegate", "delegation-conformance", inner,
		[]chat.Message{chat.NewSystemMessage("Follow the delegated instructions.")},
		chat.Options{Model: "test"},
	)
	if err != nil {
		t.Fatal(err)
	}
	input, err := program.Descriptor().EncodeInput(delegateInput{
		Summary: "Run conformance", Instructions: "Answer the delegated question completely.",
	})
	if err != nil {
		t.Fatal(err)
	}
	runtest.RunDefinitionConformance(t, runtest.DefinitionConformanceConfig{
		Definition: program.Definition(), Input: input,
		InitialTurn: scoperun.Turn{Process: (scoperun.StartID{Scope: "conformance", Key: "delegate"}).Root(), Step: 1},
	})
}
