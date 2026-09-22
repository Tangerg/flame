package agentexec

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// reportedMutationTool answers the single capability declaration Toolset owns.
// Compiling this fixture against toolset.FileMutationReporter is the point of
// the test: the capability is matched by dynamic type assertion, so a
// consumer-local copy of the method set drifts without breaking the build and
// silently classifies every call as mutating nothing.
type reportedMutationTool struct {
	toolcontract.Tool
	paths []string
	err   error
}

func (r reportedMutationTool) MutationPaths([]byte) ([]string, error) { return r.paths, r.err }

var _ toolset.FileMutationReporter = reportedMutationTool{}

func TestFileMutationScopeReadsTheCapabilityToolsetImplements(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()

	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "patch", Description: "Rewrite files.",
	}, func(context.Context, struct{}) (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		tool toolcontract.Tool
		cwd  string
		want domaintool.FileMutationScope
	}{
		{
			name: "no capability",
			tool: executable, cwd: workspace,
			want: domaintool.FileMutationNone,
		},
		{
			name: "reports no path",
			tool: reportedMutationTool{Tool: executable}, cwd: workspace,
			want: domaintool.FileMutationNone,
		},
		{
			name: "inside the workspace",
			tool: reportedMutationTool{Tool: executable, paths: []string{"nested/file.txt"}},
			cwd:  workspace,
			want: domaintool.FileMutationWithinWorkspace,
		},
		{
			name: "escapes the workspace",
			tool: reportedMutationTool{
				Tool: executable, paths: []string{filepath.Join(outside, "file.txt")},
			},
			cwd:  workspace,
			want: domaintool.FileMutationOutsideWorkspace,
		},
		{
			name: "discovery fails",
			tool: reportedMutationTool{Tool: executable, err: errors.New("unreadable patch")},
			cwd:  workspace,
			want: domaintool.FileMutationUnknown,
		},
		{
			name: "no working directory to judge against",
			tool: reportedMutationTool{Tool: executable, paths: []string{"file.txt"}},
			want: domaintool.FileMutationUnknown,
		},
		{
			name: "invalid capability chain",
			tool: &cyclicPolicyTool{Tool: executable}, cwd: workspace,
			want: domaintool.FileMutationUnknown,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := fileMutationScope(test.tool, domaintool.Arguments{}, test.cwd)
			if got != test.want {
				t.Fatalf("fileMutationScope = %q, want %q", got, test.want)
			}
		})
	}
}

type cyclicPolicyTool struct{ toolcontract.Tool }

func (c *cyclicPolicyTool) Unwrap() toolcontract.Tool { return c }

// TestWorkspaceEscapingMutationStillPromptsUnderYolo ties the capability read to
// the product rule it feeds: a mutation leaving the workspace is confirmed by a
// person even under the mode that approves everything.
func TestWorkspaceEscapingMutationStillPromptsUnderYolo(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()

	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "patch", Description: "Rewrite files.",
	}, func(context.Context, struct{}) (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}
	escaping := reportedMutationTool{
		Tool: executable, paths: []string{filepath.Join(outside, "file.txt")},
	}

	plan := (approval.ToolCallInput{
		Mode:         approval.ModeYolo,
		SafetyClass:  domaintool.SafetyClassWrite,
		FileMutation: fileMutationScope(escaping, domaintool.Arguments{}, workspace),
	}).Plan()

	if plan.Action != approval.GatePrompt ||
		plan.PromptCause != approval.PromptCauseOutsideWorkspace {
		t.Fatalf("plan = %+v, want a prompt naming the workspace escape", plan)
	}
}
