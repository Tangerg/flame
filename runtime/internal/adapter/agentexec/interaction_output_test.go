package agentexec

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestDelegatedOutputSurvivesColdRestoreWithoutReplyCache(t *testing.T) {
	image, err := media.NewBytes("image/png", []byte("image payload"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		message chat.Message
		direct  bool
	}{
		{name: "refusal and reasoning", message: chat.NewAssistantMessage(
			chat.NewReasoningPart("reasoning", []byte("opaque replay state")), chat.NewRefusalPart("cannot complete"),
		)},
		{name: "media without text", message: chat.NewAssistantMessage(chat.NewMediaPart(image))},
		{name: "direct tool results", direct: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var modelCalls, toolCalls atomic.Int32
			executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "answer", Description: "Return a direct answer."},
				func(context.Context, struct{}) (string, error) {
					toolCalls.Add(1)
					return "direct answer", nil
				})
			if err != nil {
				t.Fatal(err)
			}
			tools, err := interaction.NewToolSet(interaction.ToolSetConfig{
				Name: "output.tools", Description: "Output test tools.", Tools: []toolcontract.Tool{directResultTool{executable}},
				ImplementationDigest: agent.ComputeDigest([]byte("output-test")), ConfigurationDigest: agent.ComputeDigest([]byte("output-test")),
			})
			if err != nil {
				t.Fatal(err)
			}
			inner, err := interaction.NewDefinition(interaction.DefinitionConfig{Name: "output.interaction", Description: "Test complete output.", Tools: tools})
			if err != nil {
				t.Fatal(err)
			}
			definition, err := newDelegatedInteractionDefinition("output.delegate", inner, nil, chat.Options{Model: "test"})
			if err != nil {
				t.Fatal(err)
			}
			response := &chat.Response{Output: &chat.Output{Message: &test.message, FinishReason: chat.FinishReasonStop}}
			if test.direct {
				response = interactionToolResponse(chat.ToolCall{ID: "answer_1", Name: "answer", Arguments: `{}`}, 1, 1)
			}
			dispatcher, err := interaction.NewDispatcher(inner, interaction.DispatcherConfig{Model: chat.ModelFunc(
				func(context.Context, *chat.Request) (*chat.Response, error) {
					modelCalls.Add(1)
					return response.Clone(), nil
				})})
			if err != nil {
				t.Fatal(err)
			}
			deployment, err := agent.NewDeployment(agent.DeploymentConfig{
				Definition: definition, Dispatcher: dispatcher,
				ImplementationDigest: agent.ComputeDigest([]byte("output-test")), ConfigurationDigest: agent.ComputeDigest([]byte("output-test")),
			})
			if err != nil {
				t.Fatal(err)
			}
			store := agent.NewMemoryTreeCommitter()
			newEngine := func() *agent.Engine {
				t.Helper()
				engine, err := agent.NewEngine(agent.EngineConfig{TreeCommitter: store, DeploymentResolver: &interactionDeploymentSet{
					byRef: map[agent.DeploymentRef]agent.Deployment{tools.Deployment().DeploymentRef(): tools.Deployment()},
				}})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := engine.Close(context.Background()); err != nil {
						t.Error(err)
					}
				})
				return engine
			}
			engine := newEngine()
			input, err := deployment.Descriptor().EncodeInput(delegateInput{Summary: "test output", Instructions: "preserve complete output"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := engine.Run(t.Context(), deployment, input)
			if err != nil || result.Status() != agent.StatusCompleted {
				t.Fatalf("run: status=%s error=%v", result.Status(), err)
			}
			tree, err := engine.CaptureTree(t.Context(), result.ProcessID())
			if err != nil {
				t.Fatal(err)
			}
			tree, err = agent.ParseTreeSnapshot(tree.JSON())
			if err != nil {
				t.Fatal(err)
			}
			if err := engine.Close(t.Context()); err != nil {
				t.Fatal(err)
			}
			restored, err := newEngine().RestoreTree(t.Context(), deployment, tree)
			if err != nil {
				t.Fatal(err)
			}
			result, err = restored.Await(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			payload, _ := result.Output()
			output, err := payload.Decode[interaction.Output]()
			if err != nil || output.Validate() != nil || output.ModelCalls != 1 || modelCalls.Load() != 1 {
				t.Fatalf("restored output=%+v calls=%d error=%v", output, modelCalls.Load(), err)
			}
			if test.direct {
				if output.Source != interaction.CompletionSourceDirectToolResults || toolCalls.Load() != 1 ||
					len(output.DirectToolResults) != 1 || output.DirectToolResults[0].ID != "answer_1" {
					t.Fatalf("direct result changed: output=%+v calls=%d", output, toolCalls.Load())
				}
			} else {
				if output.Source != interaction.CompletionSourceModelResponse {
					t.Fatal("restored output lost its assistant message")
				}
				want, _ := agent.EncodePayload(test.message)
				got, err := agent.EncodePayload(*output.ModelResponse.Output.Message)
				if err != nil || !bytes.Equal(want.JSON(), got.JSON()) {
					t.Fatalf("assistant content changed: got=%s want=%s error=%v", got.JSON(), want.JSON(), err)
				}
			}
		})
	}
}
