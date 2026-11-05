package execution

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestObservedMCPFailureOffloadsWithoutChangingFailureOrEvidence(t *testing.T) {
	body := strings.Repeat("acknowledged partial failure: 界\n", 300)
	for _, test := range []struct {
		name     string
		rich     bool
		stageErr error
	}{
		{name: "plain failure"},
		{name: "structured failure", rich: true},
		{name: "plain staging failure", stageErr: errors.New("storage unavailable")},
		{name: "structured staging failure", rich: true, stageErr: errors.New("storage unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := toolcontract.NewTextOutput(body)
			if test.rich {
				output.Details = jsontext.Value(`{"partial":true,"revision":9007199254740993}`)
			}
			executable := discoverInMemoryMCPResult(t, output, true)
			store := &fakeOffloader{err: test.stageErr}
			calls := 0
			model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				calls++
				if !hasToolMessage(request.Messages) {
					return interactionToolResponse(chat.ToolCall{ID: "inspect", Name: "fixture_inspect", Arguments: `{}`}, 1, 1), nil
				}
				for _, message := range request.Messages {
					for _, part := range message.Parts {
						if part.ToolResult == nil {
							continue
						}
						result := part.ToolResult
						if !result.IsError || string(result.Output.Details) != string(output.Details) {
							t.Errorf("failure projection changed outcome or parallel data: isError=%t details=%s", result.IsError, result.Output.Details)
						}
						if test.stageErr != nil {
							if !reflect.DeepEqual(result.Output, output) {
								t.Error("failed staging replaced the original failure output")
							}
						} else if text, _ := result.Output.Text(); len(text) >= len(body) || !strings.Contains(text, store.lastStage.ID.String()) {
							t.Errorf("Scope failure bypassed the model preview: bytes=%d", len(text))
						}
					}
				}
				return interactionTextResponse("done"), nil
			})
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
				ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "inspect")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
				ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
				ToolResultStore:   store,
				ToolResultOffload: ToolResultOffloadPolicyValues{Threshold: intPointer(100), ReaderName: testToolResultReaderName},
			})
			events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
			finished := payloadsOf[runs.ToolCallFinished](events)
			if calls != 2 || len(finished) != 1 || store.calls != 1 || store.lastStage.Body != body {
				t.Fatalf("failure completion lost its single text owner: calls=%d stages=%d results=%+v", calls, store.calls, finished)
			}
			result := finished[0]
			if result.Failure == nil || result.Failure.Kind != domaintool.FailureExecution || !strings.Contains(result.Failure.Detail, "reported failure") || result.ModelResult == nil || !result.ModelResult.IsError || result.Result == nil {
				t.Fatalf("durable failure lost its diagnostic or outcome: %+v", result)
			}
			original, _, err := runtimeToolResult(output)
			if err != nil {
				t.Fatal(err)
			}
			hydrated := *result.Result
			if test.stageErr != nil {
				if result.Offload != nil {
					t.Fatal("failed staging published an unavailable offload reference")
				}
			} else {
				if result.Offload == nil || result.Offload.ModelOnly != test.rich || result.Offload.ID != store.lastStage.ID {
					t.Fatalf("failure offload changed hydration purpose: %+v", result.Offload)
				}
				hydrated, err = result.Offload.HydrateResult(result.Result, store.lastStage.Body)
				if err != nil {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(hydrated, original) {
				t.Fatal("failure offload lost original durable evidence")
			}
		})
	}
}

func TestToolResultProjectionDoesNotOffloadRejectionOrUnknownEvidence(t *testing.T) {
	output := toolcontract.NewTextOutput(strings.Repeat("observed evidence ", 300))
	store := new(fakeOffloader)
	policy, err := newToolResultOffloadPolicy(ToolResultOffloadPolicyValues{Threshold: intPointer(100), ReaderName: testToolResultReaderName})
	if err != nil {
		t.Fatal(err)
	}
	observer := observedInteractionTool{offloader: store, offloadPolicy: policy, start: interactionTestStart()}
	arguments, err := domaintool.ParseArguments(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	start := runs.ToolCallStarted{ToolName: "fixture_inspect"}
	rejected, err := toolcontract.NewFailure(toolcontract.FailureConfig{Kind: toolcontract.FailureKindRejected, Output: output})
	if err != nil {
		t.Fatal(err)
	}
	projected, err := observer.projectToolResult(t.Context(), runs.ExecutorMember{}, start, arguments, toolcontract.Output{}, rejected, nil)
	if err != nil {
		t.Fatal(err)
	}
	failure, known := toolcontract.ClassifyFailure(projected.outcome)
	if !known || failure.Kind() != toolcontract.FailureKindRejected || !reflect.DeepEqual(failure.Output(), output) || projected.metadata.Offload != nil || projected.metadata.Failure.Kind != domaintool.FailureDenied || store.calls != 0 {
		t.Fatal("rejection was offloaded or reclassified")
	}
	unknown, err := toolcontract.NewCallError(toolcontract.CallErrorConfig{Cause: context.Canceled, Evidence: output})
	if err != nil {
		t.Fatal(err)
	}
	projected, err = observer.projectToolResult(t.Context(), runs.ExecutorMember{}, start, arguments, output, unknown, nil)
	if err != unknown || projected.metadata.Result != nil || projected.metadata.Failure != nil || projected.metadata.Offload != nil || projected.outcome != nil || store.calls != 0 {
		t.Fatal("unknown evidence acquired a completed result or new failure category")
	}
}
