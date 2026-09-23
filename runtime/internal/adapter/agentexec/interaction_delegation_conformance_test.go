package agentexec

import (
	"testing"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/agenttest"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	"github.com/Tangerg/scope/core/chat"
)

// TestDelegatedDefinitionConformance runs the Agent Framework's own Definition
// suite against the one Definition Runtime implements. What it decides is what
// the framework requires of any Definition: a Descriptor that does not change
// between reads, and fresh Executions that are isolated from each other and
// deterministic for one input. Strategy behaviour beyond that stays in this
// package's own tests, which is where the framework says it belongs.
func TestDelegatedDefinitionConformance(t *testing.T) {
	inner, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name: "conformance.interaction", Description: "Delegated Interaction conformance.",
	})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := newDelegatedInteractionDefinition(
		"conformance.delegate", inner,
		[]chat.Message{chat.NewSystemMessage("Follow the delegated instructions.")},
		chat.Options{Model: "test"},
	)
	if err != nil {
		t.Fatal(err)
	}
	input, err := agent.EncodePayload(delegateInput{
		Summary:      "Run conformance",
		Instructions: "Answer the delegated question completely.",
	})
	if err != nil {
		t.Fatal(err)
	}
	agenttest.RunDefinitionConformance(t, agenttest.DefinitionConformanceConfig{
		Definition: definition,
		Input:      input,
	})
}
