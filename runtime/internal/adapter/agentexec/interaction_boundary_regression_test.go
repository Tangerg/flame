package agentexec

import (
	"context"
	"fmt"
	"iter"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestSemanticRefusalCrossesRealInterpreterAndResultCommitter(t *testing.T) {
	for _, source := range []string{"original", "hook", "authorization"} {
		t.Run(source, func(t *testing.T) {
			executions, modelCalls := 0, 0
			executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: domaintool.Shell, Description: "Execute a command."}, func(context.Context, struct {
				Command string `json:"command"`
			}) (string, error) {
				executions++
				return "unexpected", nil
			})
			if err != nil {
				t.Fatal(err)
			}
			arguments := `{"command":"   "}`
			rewritten, err := domaintool.ParseArguments(arguments)
			if err != nil {
				t.Fatal(err)
			}
			cfg := InteractionExecutorConfig{
				ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
				ToolInterpreter: toolset.NewInterpreter(nil), ToolAuthorizer: allowInteractionTools{},
			}
			switch source {
			case "hook":
				arguments = `{"command":"valid"}`
				cfg.ToolHooks = &feedbackHooks{decision: AllowToolHook(false, &rewritten)}
			case "authorization":
				arguments = `{"command":"valid"}`
				cfg.ToolAuthorizer = feedbackAuthorizer{decision: AllowToolWithArguments(rewritten)}
			}
			var committed *chat.ToolResult
			model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				modelCalls++
				if modelCalls == 1 {
					return interactionToolResponse(chat.ToolCall{ID: "refused", Name: domaintool.Shell, Arguments: arguments}, 1, 1), nil
				}
				var results []chat.ToolResult
				for _, message := range request.Messages {
					for _, part := range message.Parts {
						if part.ToolResult != nil {
							results = append(results, *part.ToolResult)
						}
					}
				}
				if len(results) != 1 || committed == nil || !reflect.DeepEqual(results[0], *committed) {
					return nil, fmt.Errorf("feedback differs from durable result: %+v", results)
				}
				return interactionTextResponse("corrected"), nil
			})
			executor := newObservedTestInteractionExecutor(t, model, cfg)
			publications := 0
			events := runInteractionHarnessWithCommit(t, executor, interactionTestStart(), func(fact runs.ExecutionFact) error {
				if batch, ok := fact.(runs.ToolResultsCommitted); ok {
					publications++
					if len(batch.Results) != 1 {
						return fmt.Errorf("results: %+v", batch.Results)
					}
					committed = batch.Results[0].ModelResult
					if batch.Results[0].Arguments != rewritten.Canonical() {
						return fmt.Errorf("effective arguments lost")
					}
				}
				return nil
			})
			ends := payloadsOf[runs.SegmentEnded](events)
			if executions != 0 || modelCalls != 2 || publications != 1 || committed == nil || !committed.IsError || committed.ID != "refused" || len(ends) != 1 || ends[0].Reason != run.OutcomeCompleted {
				t.Fatalf("executions=%d model=%d publications=%d result=%+v terminal=%+v", executions, modelCalls, publications, committed, ends)
			}
		})
	}
}

func TestCompleteModelResponseSurvivesScopeCancellation(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprint(streaming), func(t *testing.T) {
			var executor *InteractionExecutor
			model := cancelAfterResponseModel{cancel: func() {
				session := executor.sessions.snapshot()[0]
				if err := executor.RequestRootCancellation(t.Context(), session.ref, "response already received"); err != nil {
					t.Error(err)
				}
			}}
			executor = newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{StreamModelResponses: streaming})
			events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
			completed := payloadsOf[runs.ModelCallCompleted](events)
			ended := payloadsOf[runs.SegmentEnded](events)
			if len(completed) != 1 || completed[0].Message.Text() != "observed" || completed[0].ReportedUsage == nil || completed[0].ReportedUsage.PromptTokens != 2 || len(payloadsOf[runs.ModelCallFailed](events)) != 0 {
				t.Fatalf("known response lost: completed=%+v events=%+v", completed, events)
			}
			if len(ended) != 1 || ended[0].Reason != run.OutcomeCanceled {
				t.Fatalf("Scope terminal changed: %+v", ended)
			}
		})
	}
}

type cancelAfterResponseModel struct{ cancel func() }

func (m cancelAfterResponseModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	response := interactionUsageTextResponse("observed", 2, 1)
	m.cancel()
	return response, nil
}
func (m cancelAfterResponseModel) Stream(context.Context, *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return func(yield func(*chat.ResponseDelta, error) bool) {
		for delta, err := range testsupport.StreamResponse(interactionUsageTextResponse("observed", 2, 1), nil) {
			if !yield(delta, err) {
				return
			}
		}
		m.cancel()
	}
}

func TestCanceledStreamPublishesIncompleteObservationBeforeTerminal(t *testing.T) {
	var executor *InteractionExecutor
	model := canceledPrefixModel{cancel: func() {
		session := executor.sessions.snapshot()[0]
		if err := executor.RequestRootCancellation(t.Context(), session.ref, "stop incomplete response"); err != nil {
			t.Error(err)
		}
	}}
	executor = newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{StreamModelResponses: true})
	events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
	failed := payloadsOf[runs.ModelCallFailed](events)
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(failed) != 1 || failed[0].Observation.Text != "prefix" || failed[0].Observation.Reasoning != "thinking" || len(payloadsOf[runs.ModelCallCompleted](events)) != 0 {
		t.Fatalf("incomplete observation lost or promoted: %+v", events)
	}
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCanceled {
		t.Fatalf("Scope terminal changed: %+v", ended)
	}
	seenFailure := false
	for _, event := range events {
		switch event.Payload.(type) {
		case runs.ModelCallFailed:
			seenFailure = true
		case runs.SegmentEnded:
			if !seenFailure {
				t.Fatal("terminal overtook observation")
			}
		}
	}
}

type canceledPrefixModel struct{ cancel func() }

func (canceledPrefixModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return nil, fmt.Errorf("unexpected nonstreaming call")
}
func (m canceledPrefixModel) Stream(ctx context.Context, _ *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return func(yield func(*chat.ResponseDelta, error) bool) {
		if !yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewReasoningDelta("thinking", nil), chat.NewTextDelta("prefix")}}, nil) {
			return
		}
		m.cancel()
		<-ctx.Done()
		yield(nil, ctx.Err())
	}
}
