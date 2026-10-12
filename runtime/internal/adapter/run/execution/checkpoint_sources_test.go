package execution

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestCheckpointCapturesFrozenMCPReferences(t *testing.T) {
	ref := testsupport.MCPTool(testsupport.UserMCPServer("files"), "read")
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: ref.ModelName()}, func(context.Context, struct{}) (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := identifyTestManifest(toolset.Manifest{Deferred: []toolcontract.Tool{executable}}, ref)
	if err != nil {
		t.Fatal(err)
	}
	session := &interactionSession{}
	session.state.deployments = &interactionDeploymentSet{manifests: []toolset.Manifest{manifest, manifest}}
	got, err := session.checkpointMCPReferences()
	if err != nil || !reflect.DeepEqual(got, []string{ref.String()}) {
		t.Fatalf("captured references=%v, %v", got, err)
	}
}

func TestWaitingRecoveryDefersOnlyCapturedPendingSources(t *testing.T) {
	workspace := t.TempDir()
	checkpoint := captureInteractionQuestionCheckpoint(t, workspace)
	source := testsupport.UserMCPServer("files")
	var wire interactionCheckpointPayloadWire
	if err := json.Unmarshal(checkpoint.Payload, &wire); err != nil {
		t.Fatal(err)
	}
	wire.MCPReferences = new([]string{testsupport.MCPTool(source, "read").String()})
	var err error
	checkpoint.Payload, err = json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	continuation := rootInteractionWaitingContinuation(checkpoint, workspace, "exec_restore", run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}})
	trees := testTrees(t)
	for _, pending := range []mcpserver.ID{source, testsupport.UserMCPServer("unrelated")} {
		t.Run(pending.String(), func(t *testing.T) {
			executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				return nil, errors.New("recovery must not call model")
			}), InteractionExecutorConfig{
				ExecutionTrees:  trees,
				ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "ask")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{newQuestionCheckpointTool(t)}, PendingMCP: []mcpserver.ID{pending}}},
				ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
			})
			verdict, err := executor.CanResumeWaitingExecution(t.Context(), continuation)
			if err != nil || verdict.Deferred() != (pending == source) || verdict.Resumable() != (pending != source) {
				t.Fatalf("pending source verdict=%+v, %v", verdict, err)
			}
			if pending == source {
				for range 2 {
					_, err := executor.RestoreWaitingExecution(t.Context(), continuation)
					if err == nil || errors.Is(err, runs.ErrExecutorStateLost) || errors.Is(err, runs.ErrExecutionClaimed) {
						t.Fatalf("pending source lost or claimed the tree: %v", err)
					}
				}
			}
		})
	}
}

func TestCheckpointRequiresCurrentMCPDependencies(t *testing.T) {
	for _, payload := range []string{`{"tree":{}}`, `{"mcp_references":null,"tree":{}}`, `{"mcp_references":["builtIn:read"],"tree":{}}`} {
		if _, err := decodeInteractionCheckpointPayload([]byte(payload)); err == nil {
			t.Fatalf("incompatible dependencies decoded: %s", payload)
		}
	}
}
