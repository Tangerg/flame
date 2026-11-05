package execution

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"sync/atomic"
	"testing"

	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestToolResultRejectsUnrepresentableTextWithoutPanicking(t *testing.T) {
	_, observed, err := runtimeToolResult(toolcontract.NewTextOutput(string([]byte{'o', 'k', 0xff})))
	if !observed || !errors.Is(err, domaintool.ErrInvalidResult) {
		t.Fatalf("unrepresentable observed text = %t, %v", observed, err)
	}
}

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
				Revision: "output-test", Tools: []toolcontract.Tool{directResultTool{executable}},
			})
			if err != nil {
				t.Fatal(err)
			}
			response := &chat.Response{Output: &chat.Output{Message: &test.message, FinishReason: chat.FinishReasonStop}}
			if test.direct {
				response = interactionToolResponse(chat.ToolCall{ID: "answer_1", Name: "answer", Arguments: `{}`}, 1, 1)
			}
			newRuntime := func(store scoperun.Store) *scoperun.Runtime {
				t.Helper()
				engine, err := scoperun.New(t.Context(), scoperun.Config{Store: store, Build: "output-test"})
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
			engine := newRuntime(scoperun.NewMemoryStore())
			inner, err := interaction.Compile(engine.Compiler(), interaction.Config{
				Name: "output.interaction", Description: "Test complete output.", Revision: "output-test", Tools: tools,
				Model: interaction.CallModel(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
					modelCalls.Add(1)
					return response.Clone(), nil
				})),
			})
			if err != nil {
				t.Fatal(err)
			}
			program, err := compileDelegatedInteraction(engine.Compiler(), "output.delegate", "output-test", inner, nil, chat.Options{Model: "test"})
			if err != nil {
				t.Fatal(err)
			}
			handle, err := engine.Start(t.Context(), program, scoperun.StartRequest[delegateInput]{
				ID:    scoperun.StartID{Scope: "output-test", Key: "preserve-output"},
				Input: delegateInput{Summary: "test output", Instructions: "preserve complete output"},
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := handle.Await(t.Context())
			if err != nil || result.Termination().Status() != scoperun.StatusCompleted {
				t.Fatalf("run: status=%s error=%v", result.Termination().Status(), err)
			}
			history, err := engine.History(t.Context(), result.ProcessID())
			if err != nil {
				t.Fatal(err)
			}
			wire, err := json.Marshal(history)
			if err != nil {
				t.Fatal(err)
			}
			var persisted scoperun.Log
			if err := json.Unmarshal(wire, &persisted); err != nil {
				t.Fatal(err)
			}
			if err := engine.Close(t.Context()); err != nil {
				t.Fatal(err)
			}
			store := scoperun.NewMemoryStore()
			records := make([]scoperun.Record, len(persisted.Entries))
			for index, entry := range persisted.Entries {
				records[index] = entry.Record
			}
			if err := store.Append(t.Context(), scoperun.Batch{Root: persisted.Root, Records: records}); err != nil {
				t.Fatal(err)
			}
			restored, err := newRuntime(store).Recover(t.Context(), program, persisted.Root)
			if err != nil {
				t.Fatal(err)
			}
			result, err = restored.Await(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			output, err := result.Output()
			if err != nil || output.Validate() != nil || output.ModelCalls != 1 || modelCalls.Load() != 1 {
				t.Fatalf("restored output=%+v calls=%d error=%v", output, modelCalls.Load(), err)
			}
			if test.direct {
				if output.ModelResponse != nil || toolCalls.Load() != 1 ||
					len(output.DirectToolResults) != 1 || output.DirectToolResults[0].ID != "answer_1" {
					t.Fatalf("direct result changed: output=%+v calls=%d", output, toolCalls.Load())
				}
			} else {
				if output.ModelResponse == nil {
					t.Fatal("restored output lost its assistant message")
				}
				want, _ := scoperun.EncodePayload(test.message)
				got, err := scoperun.EncodePayload(*output.ModelResponse.Output.Message)
				if err != nil || !bytes.Equal(want.JSON(), got.JSON()) {
					t.Fatalf("assistant content changed: got=%s want=%s error=%v", got.JSON(), want.JSON(), err)
				}
			}
		})
	}
}
