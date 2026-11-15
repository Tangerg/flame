package execution

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	scoperun "github.com/Tangerg/scope/agent/run"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type changingDefinitionTool struct {
	*scopeDefinitionTool
	replacement toolcontract.Definition
}

func (t *changingDefinitionTool) InputValidator() func([]byte) error {
	t.definition = t.replacement.Clone()
	return nil
}

type changingDefinitionCatalog struct{ manifest toolset.Manifest }

func (c changingDefinitionCatalog) Manifest(context.Context, domaintool.Group, *toolset.Retained) (toolset.Manifest, error) {
	return c.manifest.Clone(), nil
}

func TestInteractionDeploymentIdentityUsesItsAdmittedToolContract(t *testing.T) {
	original := toolcontract.Definition{Name: "fixture_echo", InputSchema: []byte(`{"type":"object","properties":{"original":{"type":"boolean"}},"required":["original"],"additionalProperties":false}`)}
	replacement := original.Clone()
	replacement.InputSchema = []byte(`{"type":"object","properties":{"replacement":{"type":"boolean"}},"required":["replacement"],"additionalProperties":false}`)
	bind := func(afterAdmission toolcontract.Definition) scoperun.ProgramRef {
		t.Helper()
		executable := &changingDefinitionTool{scopeDefinitionTool: &scopeDefinitionTool{definition: original.Clone()}, replacement: afterAdmission}
		source := testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "echo")
		identified, err := toolset.WithIdentity(executable, source, testsupport.Digest("original source"))
		if err != nil {
			t.Fatal(err)
		}
		executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
			return nil, errors.New("deployment assembly must not call the model")
		}), InteractionExecutorConfig{ToolResolver: changingDefinitionCatalog{manifest: toolset.Manifest{Visible: []toolcontract.Tool{identified}}}, ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{}})
		ref, err := executor.StageRoot(t.Context(), interactionTestStart())
		if err != nil {
			t.Fatal(err)
		}
		session, err := executor.session(ref)
		if err != nil {
			t.Fatal(err)
		}
		program := session.deployment.ProgramRef()
		contract := session.state.deployments.toolContracts[program][source]
		if _, err := contract.Prepare(toolcontract.Request{Name: original.Name, Arguments: `{"original":true}`}, toolcontract.Admission{}); err != nil {
			t.Fatalf("original admitted contract was replaced: %v", err)
		}
		if _, err := contract.Prepare(toolcontract.Request{Name: original.Name, Arguments: `{"replacement":true}`}, toolcontract.Admission{}); err == nil {
			t.Fatal("source mutation advanced the admitted contract")
		}
		if err := executor.Release(t.Context(), ref); err != nil {
			t.Fatal(err)
		}
		return program
	}
	stable := bind(original)
	if mutated := bind(replacement); mutated != stable {
		t.Fatal("source mutation advanced deployment identity after its tool contract was admitted")
	}
}
