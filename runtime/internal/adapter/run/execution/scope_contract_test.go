package execution

import (
	corecontent "github.com/Tangerg/scope/core/content"

	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/agent/interaction"
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
				publication, attributed := executionctx.ModelCallsFromContext(ctx).(modelCalls)
				if !attributed {
					return nil, errors.New("model call has no product owner")
				}
				if publication.member.ParentID != "" {
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

type scopeOutputTool struct{ output toolcontract.Output }

func (scopeOutputTool) Definition() toolcontract.Definition {
	return toolcontract.Definition{Name: "fixture_inspect", Description: "Inspect the workspace.", InputSchema: jsontext.Value(`{"type":"object","additionalProperties":false}`)}
}

func (s scopeOutputTool) Call(context.Context, toolcontract.Invocation) (toolcontract.Result, error) {
	return toolcontract.Result{Output: s.output.Clone()}, nil
}

type scopeDefinitionTool struct{ definition toolcontract.Definition }

func (s *scopeDefinitionTool) Definition() toolcontract.Definition { return s.definition.Clone() }

func (*scopeDefinitionTool) Call(context.Context, toolcontract.Invocation) (toolcontract.Result, error) {
	return toolcontract.Result{Output: toolcontract.NewTextOutput("done")}, nil
}

func TestObservedToolPreservesBoundDefinition(t *testing.T) {
	executable := &scopeDefinitionTool{definition: scopeOutputTool{}.Definition()}
	want := executable.Definition()
	if err := want.InputSchema.Canonicalize(); err != nil {
		t.Fatal(err)
	}
	manifest, err := identifyTestManifest(toolset.Manifest{Visible: []toolcontract.Tool{executable}}, testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "inspect"))
	if err != nil {
		t.Fatal(err)
	}
	visible, _, _, _, err := wrapInteractionTools(
		t.Context(),
		manifest, &interactionSession{},
		InteractionExecutorConfig{ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{}},
		toolResultOffloadPolicy{}, runs.RootExecutionStart{},
	)
	if err != nil {
		t.Fatal(err)
	}
	executable.definition.Name = "replacement"
	executable.definition.InputSchema = jsontext.Value(`{"type":"object","required":["replacement"]}`)
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
	valid := scopeOutputTool{output: toolcontract.NewTextOutput("done")}
	var absent *scopeDefinitionTool
	for _, test := range []struct {
		name     string
		manifest toolset.Manifest
		want     error
	}{
		{"nil", toolset.Manifest{Visible: []toolcontract.Tool{nil}}, toolcontract.ErrInvalidTool},
		{"typed nil", toolset.Manifest{Deferred: []toolcontract.Tool{absent}}, toolcontract.ErrInvalidTool},
		{"invalid name", toolset.Manifest{Visible: []toolcontract.Tool{&scopeDefinitionTool{definition: toolcontract.Definition{Name: "invalid tool", InputSchema: jsontext.Value(`{}`)}}}}, toolcontract.ErrInvalidTool},
		{"invalid schema", toolset.Manifest{Deferred: []toolcontract.Tool{&scopeDefinitionTool{definition: toolcontract.Definition{Name: "invalid_schema", InputSchema: jsontext.Value(`{"type":"invalid"}`)}}}}, toolcontract.ErrInvalidTool},
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
					ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "inspect")}, manifest: test.manifest},
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
		output  toolcontract.Output
		invalid bool
		offload bool
	}{
		{name: "invalid details", output: toolcontract.Output{Details: jsontext.Value(`{"unterminated":"` + text)}, invalid: true},
		{name: "structured content", output: toolcontract.Output{Content: toolcontract.NewTextOutput(text).Content, Details: jsontext.Value(`{"revision":9007199254740993}`)}, offload: true},
		{name: "details-only fallback", output: toolcontract.Output{Details: jsontext.Value(`{"body":"` + text + `"}`)}, offload: true},
		{name: "cited text", output: toolcontract.Output{Content: []corecontent.Block{{Kind: corecontent.Text, Text: text, Citations: []corecontent.Citation{{Source: corecontent.CitationSource{Kind: corecontent.CitationSourceReference, Value: "source-1"}, Quote: "workspace observation"}}}}}},
		{name: "annotated text", output: toolcontract.Output{Content: []corecontent.Block{{Kind: corecontent.Text, Text: text, Metadata: metadata.Map{"revision": jsontext.Value(`9007199254740993`)}}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := new(fakeOffloader)
			var calls int
			model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				calls++
				if hasToolMessage(request.Messages) {
					for _, message := range request.Messages {
						for _, part := range message.Parts {
							if part.ToolResult != nil {
								if test.offload {
									got := part.ToolResult.Output
									if string(got.Details) != string(test.output.Details) || len(got.Content[0].Text) >= len(text) || !strings.Contains(got.Content[0].Text, store.lastStage.ID.String()) {
										t.Errorf("model preview lost structured output or reader: %+v", got)
									}
								} else if !reflect.DeepEqual(part.ToolResult.Output, test.output) {
									t.Errorf("protected output block changed: %+v", part.ToolResult.Output)
								}
							}
						}
					}
					return interactionTextResponse("done"), nil
				}
				return interactionToolResponse(chat.ToolCall{ID: "inspect", Name: "fixture_inspect", Arguments: `{}`}, 1, 1), nil
			})
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
				ToolResolver:    staticInteractionTools{identities: []domaintool.Ref{testsupport.MCPTool(testsupport.UserMCPServer("fixture"), "inspect")}, manifest: toolset.Manifest{Visible: []toolcontract.Tool{scopeOutputTool{output: test.output}}}},
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
			wantOffloads := 0
			if test.offload {
				wantOffloads = 1
			}
			if len(ends) != 1 || ends[0].Ending.Outcome() != want || store.calls != wantOffloads {
				t.Fatalf("tool output contract: ends=%+v offloads=%d", ends, store.calls)
			}
			if !test.invalid {
				finished := payloadsOf[runs.ToolCallFinished](events)
				if len(finished) != 1 || finished[0].ModelResult == nil {
					t.Fatalf("durable tool result lost Scope output: %+v", finished)
				}
				original, _, err := runtimeToolResult(test.output)
				if err != nil || finished[0].Result == nil || !reflect.DeepEqual(*finished[0].Result, original) {
					t.Fatalf("transcript lost its original structured result: %+v %v", finished[0], err)
				}
				if test.offload && (finished[0].Offload == nil || !finished[0].Offload.ModelOnly) {
					t.Fatal("rich output did not retain its model-only offload purpose")
				}
				if !test.offload && !reflect.DeepEqual(finished[0].ModelResult.Output, test.output) {
					t.Fatal("protected rich output changed in durable model evidence")
				}
			}
		})
	}
}
