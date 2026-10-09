package execution

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

func TestMCPMetadataProjectsLogicalCallIdentityAcrossReusedProviderIDs(t *testing.T) {
	var identities []string
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "reviews_update_review"}, func(ctx context.Context, _ struct{}) (string, error) {
		id, _ := scopemcp.RequestMetaFromContext(ctx)["io.github.tangerg.flame/invocationId"].(string)
		identities = append(identities, id)
		return "updated", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "reused", Name: "reviews_update_review", Arguments: `{}`}, 1, 1),
		interactionToolResponse(chat.ToolCall{ID: "reused", Name: "reviews_update_review", Arguments: `{}`}, 1, 1),
		interactionTextResponse("done"),
	}}
	ref := testsupport.MCPTool(testsupport.UserMCPServer("reviews"), "update_review")
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver: staticInteractionTools{identities: []domaintool.Ref{ref}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{
			identifiedPolicyTool{Tool: executable, server: "reviews", remote: "update_review"},
		}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
	starts := payloadsOf[runs.ToolCallStarted](events)
	if len(starts) != 2 || len(identities) != 2 {
		t.Fatalf("starts = %+v, metadata = %v", starts, identities)
	}
	for i, start := range starts {
		if identities[i] == "" || identities[i] != start.CallID || start.SourceCallID != "reused" {
			t.Fatalf("metadata = %q, canonical start = %+v", identities[i], start)
		}
	}
	if identities[0] == identities[1] {
		t.Fatal("different logical calls reused one backend invocation identity")
	}
}
