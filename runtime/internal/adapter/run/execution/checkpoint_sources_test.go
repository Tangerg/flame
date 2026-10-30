package execution

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestExecutionContextRequiresFrozenInputs(t *testing.T) {
	checkpoint := captureInteractionQuestionCheckpoint(t, t.TempDir())
	for _, field := range []string{"options", "instructions", "mcp_manifests", "handoff", "tool_proposals"} {
		for _, present := range []bool{false, true} {
			t.Run(field+"/"+fmt.Sprint(present), func(t *testing.T) {
				var document map[string]jsontext.Value
				if err := json.Unmarshal(checkpoint.Payload, &document); err != nil {
					t.Fatal(err)
				}
				if present {
					document[field] = jsontext.Value("null")
				} else {
					delete(document, field)
				}
				payload, err := json.Marshal(document)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := decodeInteractionCheckpointPayload(payload); err == nil {
					t.Fatal("missing frozen inputs became defaults")
				}
			})
		}
	}
}

func TestCheckpointCapturesMCPManifestsPerGroup(t *testing.T) {
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
	session.state.deployments = &interactionDeploymentSet{manifests: map[domaintool.Group]toolset.Manifest{domaintool.GroupRoot: {}, domaintool.GroupDelegated: manifest}}
	got, err := session.checkpointMCPManifests()
	if err != nil || !reflect.DeepEqual(got, map[domaintool.Group][]string{domaintool.GroupRoot: {}, domaintool.GroupDelegated: {ref.String()}}) {
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
	(*wire.MCPManifests)[domaintool.GroupRoot] = append((*wire.MCPManifests)[domaintool.GroupRoot], testsupport.MCPTool(source, "read").String())
	var err error
	checkpoint.Payload, err = json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	continuation := rootInteractionExecutionContinuation(checkpoint, workspace, "exec_restore", run.Capabilities{InterruptKinds: []run.InterruptKind{run.InterruptQuestion}})
	trees := testTrees(t)
	for _, pending := range []mcpserver.ID{source, testsupport.UserMCPServer("unrelated")} {
		t.Run(pending.String(), func(t *testing.T) {
			executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				return nil, errors.New("recovery must not call model")
			}), InteractionExecutorConfig{
				ExecutionTrees:  trees,
				ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "ask")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{newQuestionCheckpointTool(t)}, PendingMCP: []mcpserver.ID{pending}}},
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

// The root and delegated catalog may be captured on opposite sides of a
// connection publication; neither can acquire the other's later authority.
func TestWaitingRecoveryKeepsMCPManifestsPerGroup(t *testing.T) {
	workspace := t.TempDir()
	question := newQuestionCheckpointTool(t)
	memory := testsupport.MCPTool(testsupport.UserMCPServer("memory"), "read")
	schedules := testsupport.MCPTool(testsupport.UserMCPServer("schedules"), "read")
	makeTool := func(ref domaintool.Ref) toolcontract.Tool {
		executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: ref.ModelName()}, func(context.Context, struct{}) (string, error) { return "unused", nil })
		if err != nil {
			t.Fatal(err)
		}
		identified, err := toolset.WithIdentity(executable, ref, testsupport.ToolFingerprint(ref))
		if err != nil {
			t.Fatal(err)
		}
		return identified
	}
	memoryTool, schedulesTool := makeTool(memory), makeTool(schedules)
	catalog := &reconnectingGroupTools{questionRef: testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "ask"), byGroup: map[domaintool.Group]toolset.Manifest{
		domaintool.GroupRoot:      {Visible: []toolcontract.Tool{question}},
		domaintool.GroupDelegated: {Visible: []toolcontract.Tool{question}, Deferred: []toolcontract.Tool{memoryTool}},
	}}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "ask_call", Name: "fixture_ask", Arguments: `{}`}, 1, 1),
		interactionUsageTextResponse("continued after reconnect", 1, 1),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{ToolResolver: catalog, ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{}})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []run.InterruptKind{run.InterruptQuestion}
	start.ChildRunAdmissionEnabled = true
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	_, barrier := observeInteractionUntilWaiting(t, executor, ref, func() error { return executor.BeginRoot(t.Context(), ref) })
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	for _, group := range []domaintool.Group{domaintool.GroupRoot, domaintool.GroupDelegated} {
		catalog.byGroup[group] = toolset.Manifest{Visible: []toolcontract.Tool{question}, Deferred: []toolcontract.Tool{memoryTool, schedulesTool}}
	}
	continuation := rootInteractionExecutionContinuation(barrier.Checkpoint(), workspace, ref.ExecutorID, run.Capabilities{InterruptKinds: []run.InterruptKind{run.InterruptQuestion}, ChildRuns: true})
	verdict, err := executor.CanResumeWaitingExecution(t.Context(), continuation)
	if err != nil || !verdict.Resumable() {
		t.Fatalf("late connection changed captured waiting authority: %+v, %v", verdict, err)
	}
	if _, err := executor.StageAcceptedContinuation(t.Context(), continuation); err != nil {
		t.Fatal(err)
	}
	sequence, err := observeTestInteraction(t, executor, t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	events := collectInteractionEvents(sequence)
	binding := barrier.Interruptions()[0]
	if err := executor.BeginContinuation(t.Context(), ref, []runs.InterruptAnswer{{InterruptItemID: "item_question", MemberID: binding.MemberID, RequestID: binding.RequestID, Resolution: testsupport.QuestionAnswer(t, [][]string{{"yes"}})}}, nil); err != nil {
		t.Fatal(err)
	}
	observed := <-events
	ended := payloadsOf[runs.SegmentEnded](observed)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCompleted || len(payloadsOf[runs.ModelCallCompleted](observed)) != 1 {
		t.Fatalf("restored waiting lifecycle = %#v", observed)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
}

type reconnectingGroupTools struct {
	questionRef domaintool.Ref
	byGroup     map[domaintool.Group]toolset.Manifest
}

func (r *reconnectingGroupTools) Manifest(ctx context.Context, group domaintool.Group, retained *toolset.Retained) (toolset.Manifest, error) {
	return (staticInteractionTools{manifest: r.byGroup[group], identities: []domaintool.Ref{r.questionRef}}).Manifest(ctx, group, retained)
}

func TestCheckpointRejectsUncapturedMCPManifests(t *testing.T) {
	ref := testsupport.MCPTool(testsupport.UserMCPServer("files"), "read").String()
	for name, values := range map[string]map[domaintool.Group][]string{
		"missing root":       {domaintool.GroupDelegated: {}},
		"unknown group":      {domaintool.GroupRoot: {}, "other": {}},
		"missing references": {domaintool.GroupRoot: nil},
		"non MCP":            {domaintool.GroupRoot: {"builtIn:read"}},
		"duplicate":          {domaintool.GroupRoot: {ref, ref}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeMCPManifests(values); err == nil {
				t.Fatal("uncaptured or malformed authority was admitted")
			}
		})
	}
}

func TestCheckpointRequiresExplicitModelProviderProjection(t *testing.T) {
	checkpoint := captureInteractionQuestionCheckpoint(t, t.TempDir())
	var document map[string]jsontext.Value
	if err := json.Unmarshal(checkpoint.Payload, &document); err != nil {
		t.Fatal(err)
	}
	if _, present := document["model_provider"]; !present {
		t.Fatal("model provider projection was omitted")
	}
	delete(document, "model_provider")
	payload, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeInteractionCheckpointPayload(payload); err == nil {
		t.Fatal("missing model construction was restored as a fresh choice")
	}
}
