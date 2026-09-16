package agentexec

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type capturingToolAuthorizer struct{ request *ToolAuthorizationRequest }

func (c capturingToolAuthorizer) AuthorizeTool(
	_ context.Context,
	request ToolAuthorizationRequest,
) (ToolAuthorizationDecision, error) {
	*c.request = request
	return AllowTool(), nil
}

func (c capturingToolAuthorizer) ResolveToolApproval(
	_ context.Context,
	request ToolAuthorizationRequest,
	_ runs.ApprovalPrompt,
	_ interrupt.Resolution,
) (ToolAuthorizationDecision, error) {
	*c.request = request
	return AllowTool(), nil
}

// TestDurableProjectStateIsAddressedByTheWorkspace pins the rule that separates
// the two directories a Run carries. Tools operate in the execution directory,
// which for an isolated Run is a scratch copy about to be discarded; everything
// the product keeps afterwards — a remembered approval, a mined skill,
// consolidated memory — is project knowledge and is addressed by the Session's
// workspace, or it is written somewhere that stops existing.
func TestDurableProjectStateIsAddressedByTheWorkspace(t *testing.T) {
	workspace := t.TempDir()
	execution := t.TempDir()

	var maintenanceInput RunMaintenanceInput
	var authorized ToolAuthorizationRequest

	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "write", Description: "Write a file.",
	}, func(context.Context, struct{}) (string, error) { return "wrote", nil })
	if err != nil {
		t.Fatal(err)
	}
	model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if hasToolMessage(request.Messages) {
			return interactionTextResponse("done"), nil
		}
		return interactionToolBatchResponse(
			[]chat.ToolCall{{ID: "write_1", Name: "write", Arguments: `{}`}}, 1, 1,
		), nil
	})
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver: staticInteractionTools{
			manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}},
		},
		ToolInterpreter: testInteractionToolInterpreter{},
		ToolAuthorizer:  capturingToolAuthorizer{request: &authorized},
		Maintenance:     fixedRunMaintenance{input: &maintenanceInput},
	})

	start := interactionTestStart()
	start.CWD = execution
	start.WorkspaceCWD = workspace
	start.Isolated = true
	runInteractionHarness(context.Background(), t, executor, start, nil)

	if authorized.WorkspaceCWD != workspace {
		t.Fatalf("remembered-approval scope = %q, want the workspace %q",
			authorized.WorkspaceCWD, workspace)
	}
	if maintenanceInput.WorkspaceCWD != workspace {
		t.Fatalf("Run maintenance scope = %q, want the workspace %q",
			maintenanceInput.WorkspaceCWD, workspace)
	}
}
