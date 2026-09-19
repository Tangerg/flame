package agentexec

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
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

func TestFailedStreamRetainsPrefixWhenPreviewQueueIsFull(t *testing.T) {
	emitted := make(chan struct{})
	model := overflowingFailedModel{emitted: emitted}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{StreamModelResponses: true})
	ref, err := executor.StageRoot(t.Context(), interactionTestStart())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := executor.Release(context.Background(), ref); err != nil {
			t.Error(err)
		}
	}()
	session, err := executor.session(ref)
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := observeTestInteraction(t, executor, t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := executor.BeginRoot(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	// Admit the provider, then stop consuming until all preview delivery has run.
	start := <-session.lifetime.events
	commit, ok := start.Payload.(runs.ExecutionFactCommit)
	if !ok {
		t.Fatalf("first event=%T", start.Payload)
	}
	commit.Complete(nil)
	<-emitted
	if err := session.engine.FlushDeltas(t.Context()); err != nil {
		t.Fatal(err)
	}
	var failed []runs.ModelCallFailed
	previews := 0
	for event := range sequence {
		if commit, ok := event.Payload.(runs.ExecutionFactCommit); ok {
			event.Payload = commit.Fact()
			commit.Complete(nil)
		}
		switch fact := event.Payload.(type) {
		case runs.MessageDelta:
			previews++
		case runs.ModelCallFailed:
			failed = append(failed, fact)
		}
	}
	if previews >= 2048 || len(failed) != 1 || failed[0].Observation.Text != strings.Repeat("x", 2048) {
		t.Fatalf("preview loss changed authoritative prefix: previews=%d failures=%+v", previews, failed)
	}
}

type overflowingFailedModel struct{ emitted chan struct{} }

func (overflowingFailedModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return nil, errors.New("unexpected call")
}
func (m overflowingFailedModel) Stream(context.Context, *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return func(yield func(*chat.ResponseDelta, error) bool) {

		for range 2048 {
			if !yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("x")}}, nil) {
				return
			}
		}
		// Signal before the failure boundary waits for its authoritative receipt.
		close(m.emitted)

		yield(nil, errors.New("stream lost"))
	}
}
