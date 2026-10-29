package execution

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type capturingToolAuthorizer struct{ request *ToolAuthorizationRequest }

func (c capturingToolAuthorizer) AuthorizeTool(
	_ context.Context,
	request ToolAuthorizationRequest,
) (ToolAuthorizationDecision, error) {
	*c.request = request
	return allowTool(), nil
}

func (c capturingToolAuthorizer) ResolveToolApproval(
	_ context.Context,
	request ToolAuthorizationRequest,
	_ runs.ApprovalPrompt,
	_ run.InterruptResolution,
) (ToolAuthorizationDecision, error) {
	*c.request = request
	return allowTool(), nil
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
		ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "write")},
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

type scopeCapturingHookResolver struct{ resolvedAt *string }

func (s scopeCapturingHookResolver) For(_ context.Context, cwd string) (*apphooks.Bound, error) {
	*s.resolvedAt = cwd
	return nil, nil
}

type scopeCapturingMemorySearch struct{ searchedAt *string }

func (s scopeCapturingMemorySearch) Search(
	_ context.Context,
	project, _ string,
	_ int,
) ([]MemoryPromptItem, error) {
	*s.searchedAt = project
	return nil, nil
}

type memoryTargetRead struct {
	scope   memorycontract.ItemsV2InputScope
	project string
}

type scopeCapturingPinnedMemory struct {
	workspace string
	reads     *[]memoryTargetRead
}

func (s scopeCapturingPinnedMemory) Items(_ context.Context, scope memorycontract.ItemsV2InputScope, project string) ([]MemoryPromptItem, error) {
	*s.reads = append(*s.reads, memoryTargetRead{scope, project})
	content := ""
	if scope == memorycontract.ItemsV2InputScopeProject && project == s.workspace {
		content = "project pinned fact"
	}
	if scope == memorycontract.ItemsV2InputScopeUser && project == "" {
		content = "user pinned fact"
	}
	if content == "" {
		return nil, nil
	}
	return []MemoryPromptItem{testMemoryPromptItem(MemoryPromptItem{Content: content, Pinned: true})}, nil
}

// TestWorkingContextResolvesHooksAndMemoryAgainstTheWorkspace pins the other
// half of the same rule. Hook discovery decides which hooks exist and whether
// the project was trusted to run them, and recall reads memory consolidation
// wrote: an isolated copy holds neither that trust grant nor that memory, so
// resolving against it drops a user's configured hooks silently.
func TestWorkingContextResolvesHooksAndMemoryAgainstTheWorkspace(t *testing.T) {
	workspace := t.TempDir()
	execution := t.TempDir()
	for path, content := range map[string]string{workspace: "workspace instruction", execution: "isolated execution instruction"} {
		if err := os.WriteFile(filepath.Join(path, "AGENTS.md"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, cwd, doc, excluded string }{
		{"ordinary", workspace, "workspace instruction", "isolated execution instruction"},
		{"isolated", execution, "isolated execution instruction", "workspace instruction"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var hooksResolvedAt, memorySearchedAt string
			var reads []memoryTargetRead
			composer := newTestWorkingContextComposer(t, WorkingContextConfig{
				Hooks:             scopeCapturingHookResolver{resolvedAt: &hooksResolvedAt},
				AgentMemorySearch: scopeCapturingMemorySearch{searchedAt: &memorySearchedAt},
				AgentMemory:       scopeCapturingPinnedMemory{workspace: workspace, reads: &reads},
			})
			messages, err := composer.ComposeWorkingContext(t.Context(), runs.WorkingContextInput{
				SessionID: "session:one", CWD: tc.cwd, WorkspaceCWD: workspace,
				PromptText: "question", Seed: []chat.Message{chat.NewUserMessage(chat.NewTextPart("question"))},
			})
			if err != nil {
				t.Fatalf("ComposeWorkingContext: %v", err)
			}
			if hooksResolvedAt != workspace || memorySearchedAt != workspace {
				t.Fatalf("durable scope: hooks=%q recall=%q", hooksResolvedAt, memorySearchedAt)
			}
			if !slices.Equal(reads, []memoryTargetRead{{memorycontract.ItemsV2InputScopeProject, workspace}, {memorycontract.ItemsV2InputScopeUser, ""}}) {
				t.Fatalf("pinned targets = %+v", reads)
			}
			instructions := messages[0].Text()
			for _, text := range []string{"project pinned fact", "user pinned fact", tc.doc} {
				if !strings.Contains(instructions, text) {
					t.Fatalf("missing context %q", text)
				}
			}
			if strings.Contains(instructions, tc.excluded) {
				t.Fatalf("agent documents escaped execution directory: %q", tc.excluded)
			}
		})
	}
}
