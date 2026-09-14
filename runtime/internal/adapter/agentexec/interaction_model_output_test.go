package agentexec

import (
	"context"
	"errors"
	"fmt"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"iter"
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func TestFirstModelOutputExcludesStreamBookkeeping(t *testing.T) {
	for _, test := range []struct {
		name   string
		delta  chat.ResponseDelta
		output bool
	}{
		{name: "metadata", delta: chat.ResponseDelta{Metadata: &chat.ResponseMetadata{Model: "model"}}},
		{name: "usage", delta: chat.ResponseDelta{Metadata: &chat.ResponseMetadata{Usage: &chat.Usage{InputTokens: 3}}}},
		{name: "finish", delta: chat.ResponseDelta{FinishReason: chat.FinishReasonStop}},
		{name: "opaque reasoning", delta: chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewReasoningDelta("", []byte("opaque"))}}},
		{name: "text", delta: chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta(" ")}}, output: true},
		{name: "reasoning", delta: chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewReasoningDelta("thinking", nil)}}, output: true},
		{name: "refusal", delta: chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewRefusalDelta("refused")}}, output: true},
		{name: "tool name", delta: chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewToolCallDelta(chat.ToolCallDelta{ID: "call", Name: "read"})}}, output: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.delta.Validate(); err != nil {
				t.Fatal(err)
			}
			if got := hasModelOutput(&test.delta); got != test.output {
				t.Fatalf("model output = %v, want %v", got, test.output)
			}
		})
	}
}

func TestFailedStreamRetainsOnlyObservedFirstOutputLatency(t *testing.T) {
	for _, output := range []bool{false, true} {
		t.Run(fmt.Sprint(output), func(t *testing.T) {
			executor := newObservedTestInteractionExecutor(t, failedOutputModel{output: output}, InteractionExecutorConfig{StreamModelResponses: true})
			events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
			failed := payloadsOf[runs.ModelCallFailed](events)
			if len(failed) != 1 {
				t.Fatalf("failed calls = %d", len(failed))
			}
			latency := failed[0].FirstOutputLatencyMillis
			if (latency != nil) != output {
				t.Fatalf("observed latency = %v, output = %v", latency, output)
			}
			if latency != nil && *latency < 0 {
				t.Fatal("negative latency")
			}
		})
	}
}

type failedOutputModel struct{ output bool }

func (failedOutputModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return nil, errors.New("unexpected nonstreaming call")
}

func (m failedOutputModel) Stream(context.Context, *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return func(yield func(*chat.ResponseDelta, error) bool) {
		if !yield(&chat.ResponseDelta{Metadata: &chat.ResponseMetadata{Model: "model"}}, nil) {
			return
		}
		if m.output && !yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("partial")}}, nil) {
			return
		}
		yield(nil, errors.New("provider stream failed"))
	}
}
