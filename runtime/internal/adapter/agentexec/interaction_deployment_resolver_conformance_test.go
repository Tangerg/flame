package agentexec

import (
	"context"
	"errors"
	"testing"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/agenttest"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	"github.com/Tangerg/scope/core/chat"
)

// TestInteractionDeploymentSetResolverConformance runs the Agent Framework's own
// resolver suite against the set Runtime installs as the Engine's
// DeploymentResolver. The rules it decides — exact reference matching, no
// fallback by name, rejection of unknown references, and lookups independent of
// order, repetition, and concurrency — are stated in the framework's prose and
// checked by no compiler, so the framework's suite is what verifies them.
func TestInteractionDeploymentSetResolverConformance(t *testing.T) {
	// Two deployments that share a name and differ only in their digests: the
	// suite derives its name-collision probe from this pair, which is what
	// catches a lookup keyed by anything less than the complete reference.
	first := conformanceDeployment(t, "conformance.resolver", "first")
	second := conformanceDeployment(t, "conformance.resolver", "second")

	agenttest.RunDeploymentResolverConformance(t, agenttest.DeploymentResolverConformanceConfig{
		Resolver: &interactionDeploymentSet{byRef: map[agent.DeploymentRef]agent.Deployment{
			first.DeploymentRef():  first,
			second.DeploymentRef(): second,
		}},
		Resolvable: []agent.Deployment{first, second},
	})
}

func conformanceDeployment(t *testing.T, name, variant string) agent.Deployment {
	t.Helper()
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name: name, Description: "Deployment resolver conformance.",
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := interaction.NewDispatcher(definition, interaction.DispatcherConfig{
		Model: chat.ModelFunc(func(_ context.Context, _ *chat.Request) (*chat.Response, error) {
			return nil, errors.New("resolver conformance never calls the model")
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition:           definition,
		Dispatcher:           dispatcher,
		ImplementationDigest: agent.ComputeDigest([]byte(variant)),
		ConfigurationDigest:  agent.ComputeDigest([]byte(variant)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return deployment
}
