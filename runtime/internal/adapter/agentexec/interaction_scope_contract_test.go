package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/metadata"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestDelegateConsumesAdvertisedInputContract(t *testing.T) {
	for _, test := range []struct {
		name     string
		input    delegateInput
		rejected bool
	}{
		{name: "character limit", input: delegateInput{Summary: strings.Repeat("界", 80), Instructions: "Inspect the workspace."}},
		{name: "formatted instructions", input: delegateInput{Summary: "inspect workspace", Instructions: "\nInspect the workspace.\n"}},
		{name: "excess characters", input: delegateInput{Summary: strings.Repeat("界", 81), Instructions: "Inspect the workspace."}, rejected: true},
		{name: "untrimmed summary", input: delegateInput{Summary: " inspect workspace ", Instructions: "Inspect the workspace."}, rejected: true},
		{name: "empty instructions", input: delegateInput{Summary: "inspect workspace"}, rejected: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			arguments, err := json.Marshal(test.input)
			if err != nil {
				t.Fatal(err)
			}
			var childCalls atomic.Int32
			model := chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
				invocation, _ := interaction.ModelInvocationFromContext(ctx)
				if !invocation.Relation().IsRoot() {
					childCalls.Add(1)
					if got := request.Messages[len(request.Messages)-1].Text(); got != test.input.Instructions {
						t.Errorf("child instructions = %q, want %q", got, test.input.Instructions)
					}
					return interactionTextResponse("child completed"), nil
				}
				if hasToolMessage(request.Messages) {
					for _, message := range request.Messages {
						for _, part := range message.Parts {
							if part.ToolResult != nil && part.ToolResult.IsError != test.rejected {
								t.Errorf("delegate error = %v, want %v", part.ToolResult.IsError, test.rejected)
							}
						}
					}
					return interactionTextResponse("root completed"), nil
				}
				return interactionToolResponse(chat.ToolCall{ID: "delegate", Name: "delegate_task", Arguments: string(arguments)}, 1, 1), nil
			})
			wantChildren := 1
			if test.rejected {
				wantChildren = 0
			}
			result := startDelegateTree(t, model, "delegate the inspection").finish(t, 1+wantChildren)
			result.assertAllRunsCompleted(t)
			if childCalls.Load() != int32(wantChildren) {
				t.Fatalf("child model calls = %d, want %d", childCalls.Load(), wantChildren)
			}
		})
	}
}

func TestCheckpointWireRejectsFieldAliases(t *testing.T) {
	for _, payload := range []string{
		`{"Options":{}}`,
		`{"members":[{"Member_ID":"process:child","models":[]}]}`,
		`{"pending_continuation":{"member_id":"process:root","item_id":"item_1","Content":[]}}`,
	} {
		if _, err := decodeInteractionCheckpointWire([]byte(payload)); err == nil {
			t.Errorf("checkpoint accepted field alias: %s", payload)
		}
	}
}

type scopeOutputTool struct{ output chat.ToolOutput }

func (scopeOutputTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: "inspect", Description: "Inspect the workspace.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}
}

func (s scopeOutputTool) Call(context.Context, toolcontract.Invocation) (chat.ToolOutput, error) {
	return s.output.Clone(), nil
}

type scopeDefinitionTool struct{ definition chat.ToolDefinition }

func (s *scopeDefinitionTool) Definition() chat.ToolDefinition { return s.definition.Clone() }

func (*scopeDefinitionTool) Call(context.Context, toolcontract.Invocation) (chat.ToolOutput, error) {
	return chat.NewTextToolOutput("done"), nil
}

func TestObservedToolPreservesBoundDefinition(t *testing.T) {
	executable := &scopeDefinitionTool{definition: scopeOutputTool{}.Definition()}
	want := executable.Definition()
	visible, _, err := wrapInteractionTools(
		toolset.Manifest{Visible: []toolcontract.Tool{executable}}, nil,
		InteractionExecutorConfig{ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{}},
		toolResultOffloadPolicy{}, runs.RootExecutionStart{},
	)
	if err != nil {
		t.Fatal(err)
	}
	executable.definition.Name = "replacement"
	executable.definition.InputSchema = json.RawMessage(`{"type":"object","required":["replacement"]}`)
	got := visible[0].Definition()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("observed Tool changed its admitted definition: got=%+v want=%+v", got, want)
	}
	got.InputSchema[0] = '!'
	if !reflect.DeepEqual(visible[0].Definition(), want) {
		t.Fatal("editing an advertised definition changed the bound contract")
	}
}

func TestInteractionManifestUsesScopeAdmission(t *testing.T) {
	valid := scopeOutputTool{output: chat.NewTextToolOutput("done")}
	var absent *scopeDefinitionTool
	for _, test := range []struct {
		name     string
		manifest toolset.Manifest
		want     error
	}{
		{"nil", toolset.Manifest{Visible: []toolcontract.Tool{nil}}, toolcontract.ErrInvalidTool},
		{"typed nil", toolset.Manifest{Deferred: []toolcontract.Tool{absent}}, toolcontract.ErrInvalidTool},
		{"invalid name", toolset.Manifest{Visible: []toolcontract.Tool{&scopeDefinitionTool{definition: chat.ToolDefinition{Name: "invalid tool", InputSchema: json.RawMessage(`{}`)}}}}, toolcontract.ErrInvalidTool},
		{"invalid schema", toolset.Manifest{Deferred: []toolcontract.Tool{&scopeDefinitionTool{definition: chat.ToolDefinition{Name: "invalid_schema", InputSchema: json.RawMessage(`{"type":"invalid"}`)}}}}, toolcontract.ErrInvalidTool},
		{"visible collision", toolset.Manifest{Visible: []toolcontract.Tool{valid, valid}}, interaction.ErrInvalidToolSet},
		{"deferred collision", toolset.Manifest{Deferred: []toolcontract.Tool{valid, valid}}, interaction.ErrInvalidToolSet},
		{"cross visibility collision", toolset.Manifest{Visible: []toolcontract.Tool{valid}, Deferred: []toolcontract.Tool{valid}}, interaction.ErrInvalidToolSet},
	} {
		t.Run(test.name, func(t *testing.T) {
			executor := newObservedTestInteractionExecutor(t,
				chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
					t.Fatal("invalid manifest reached model dispatch")
					return nil, nil
				}),
				InteractionExecutorConfig{
					ToolResolver:    staticInteractionTools{manifest: test.manifest},
					ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
				},
			)
			if _, err := executor.StageRoot(t.Context(), interactionTestStart()); !errors.Is(err, test.want) {
				t.Fatalf("StageRoot error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestToolOffloadPreservesScopeOutputContract(t *testing.T) {
	text := strings.Repeat("workspace observation ", 50)
	for _, test := range []struct {
		name    string
		output  chat.ToolOutput
		invalid bool
	}{
		{name: "invalid details", output: chat.ToolOutput{Details: json.RawMessage(`{"unterminated":"` + text)}, invalid: true},
		{name: "structured content", output: chat.ToolOutput{Content: chat.NewTextToolOutput(text).Content, Details: json.RawMessage(`{"revision":9007199254740993}`)}},
		{name: "cited text", output: chat.ToolOutput{Content: []chat.ToolContent{{Kind: chat.PartText, Text: text, Citations: []chat.Citation{{Source: chat.CitationSource{Kind: chat.CitationSourceReference, Value: "source-1"}, Quote: "workspace observation"}}}}}},
		{name: "annotated text", output: chat.ToolOutput{Content: []chat.ToolContent{{Kind: chat.PartText, Text: text, Metadata: metadata.Map{"revision": json.RawMessage(`9007199254740993`)}}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := new(fakeOffloader)
			var calls int
			model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				calls++
				if hasToolMessage(request.Messages) {
					for _, message := range request.Messages {
						for _, part := range message.Parts {
							if part.ToolResult != nil && !reflect.DeepEqual(part.ToolResult.Output, test.output) {
								t.Errorf("model received a lossy tool result: %+v", part.ToolResult.Output)
							}
						}
					}
					return interactionTextResponse("done"), nil
				}
				return interactionToolResponse(chat.ToolCall{ID: "inspect", Name: "inspect", Arguments: `{}`}, 1, 1), nil
			})
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
				ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{scopeOutputTool{output: test.output}}}},
				ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
				ToolResultStore:   store,
				ToolResultOffload: ToolResultOffloadPolicyValues{Threshold: intPointer(100), ReaderName: testToolResultReaderName},
			})
			events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
			ends := payloadsOf[runs.SegmentEnded](events)
			want := run.OutcomeCompleted
			if test.invalid {
				want = run.OutcomeLost
				if calls != 1 {
					t.Errorf("invalid result continued the model: %d calls", calls)
				}
			}
			if len(ends) != 1 || ends[0].Reason != want || store.calls != 0 {
				t.Fatalf("tool output contract: ends=%+v offloads=%d", ends, store.calls)
			}
			if !test.invalid {
				finished := payloadsOf[runs.ToolCallFinished](events)
				if len(finished) != 1 || finished[0].ModelResult == nil || !reflect.DeepEqual(finished[0].ModelResult.Output, test.output) {
					t.Fatalf("durable tool result lost Scope output: %+v", finished)
				}
			}
		})
	}
}
