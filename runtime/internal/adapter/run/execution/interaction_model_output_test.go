package execution

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func TestInteractionBillsPersistedEndpointWhenOuterResultIsMissingOrChanged(t *testing.T) {
	for _, withhold := range []bool{true, false} {
		t.Run(fmt.Sprint(withhold), func(t *testing.T) {
			db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "model-evidence.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			journal := sqlite.NewModelInvocationStore(db)
			provider := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				return interactionUsageTextResponse("physical answer", 12, 3), nil
			})
			recorded, err := modeladapter.NewObservedResolver(interactionChatResolver(provider, nil), fixtureEndpointJournal{ModelInvocationStore: journal, db: db}, func(string, string, *chat.Usage) accounting.Cost { value, _ := accounting.NewCost(0.25); return value })
			if err != nil {
				t.Fatal(err)
			}
			outer := interactionChatResolverFunc(func(ctx context.Context, selection modelref.Selection) (modeladapter.ResolvedChat, error) {
				resolved, err := recorded.ResolveChat(ctx, selection)
				if err != nil {
					return modeladapter.ResolvedChat{}, err
				}
				return modeladapter.NewResolvedChat(chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
					response, err := resolved.Model().Call(ctx, request)
					if err != nil {
						return response, err
					}
					if withhold {
						return nil, errors.New("outer result withheld")
					}
					changed := response.Clone()
					changed.Metadata.Usage = &chat.Usage{InputTokens: 999, OutputTokens: 999}
					return changed, nil
				}), nil)
			})
			executor, err := newTestConfiguredInteractionExecutor(t, InteractionExecutorConfig{Lifetime: t.Context(), BuildID: interactionTestBuildID, ChatResolver: outer, ModelEvidence: journal})
			if err != nil {
				t.Fatal(err)
			}
			events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
			for _, fact := range payloadsOf[runs.ModelCallCompleted](events) {
				if usd, known := fact.Cost.USD(); !known || usd != 0.25 {
					t.Fatal("outer completion replaced source price")
				}
			}
			for _, fact := range payloadsOf[runs.ModelCallFailed](events) {
				if fact.Accounting != nil {
					if usd, known := fact.Accounting.Cost.USD(); !known || usd != 0.25 {
						t.Fatal("outer failure lost source price")
					}
				}
			}
			if withhold {
				failed := payloadsOf[runs.ModelCallFailed](events)
				if len(failed) != 1 || failed[0].Accounting == nil || failed[0].Accounting.ReportedUsage.InputTokens != 12 || failed[0].Accounting.ReportedUsage.OutputTokens != 3 || failed[0].Accounting.Steps != 1 || failed[0].KnownResponse.Text() != "physical answer" || len(payloadsOf[runs.ModelCallCompleted](events)) != 0 {
					t.Fatalf("withheld result lost or duplicated physical usage: %+v", failed)
				}
			} else {
				completed := payloadsOf[runs.ModelCallCompleted](events)
				if len(completed) != 1 || completed[0].ReportedUsage.InputTokens != 12 || completed[0].ReportedUsage.OutputTokens != 3 || completed[0].Steps != 1 {
					t.Fatalf("outer projection originated usage: %+v", completed)
				}
			}
		})
	}
}

func TestFailedOuterModelRetainsKnownResponseAndAccounting(t *testing.T) {
	message := chat.NewAssistantMessage(chat.NewTextPart("known inner answer"), chat.NewReasoningPart("basis", nil))
	response := &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: "served-model", Usage: &chat.Usage{InputTokens: 12, OutputTokens: 3, CacheReadInputTokens: new(int64(0))}}}
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return response, errors.New("outer observer failed")
	})
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{StreamModelResponses: false})
	events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
	failed := payloadsOf[runs.ModelCallFailed](events)
	if len(failed) != 1 || !reflect.DeepEqual(failed[0].KnownResponse, response) || failed[0].Accounting == nil || !reflect.DeepEqual(failed[0].Accounting.ReportedUsage, response.Metadata.Usage) || failed[0].Accounting.Steps != 1 || failed[0].Observation.Text != "known inner answer" || failed[0].Observation.Reasoning != "basis" || len(payloadsOf[runs.ModelCallCompleted](events)) != 0 {
		t.Fatalf("known inner evidence discarded or promoted: %+v", failed)
	}
	ends := payloadsOf[runs.SegmentEnded](events)
	if len(ends) != 1 || ends[0].Reason != run.OutcomeFailed {
		t.Fatalf("outer failure became success: %+v", ends)
	}
}

func TestFailedOuterStreamRetainsCompletedInnerResponse(t *testing.T) {
	message := chat.NewAssistantMessage(chat.NewTextPart("known streamed answer"))
	response := &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: "served-model", Usage: &chat.Usage{InputTokens: 12, OutputTokens: 3}}}
	model := struct {
		chat.Model
		chat.Streamer
	}{
		Model: chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
			return nil, errors.New("unexpected unary")
		}),
		Streamer: chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
			return func(yield func(*chat.ResponseDelta, error) bool) {
				for delta, err := range testsupport.StreamResponse(response, nil) {
					if !yield(delta, err) {
						return
					}
				}
				yield(nil, errors.New("outer observer failed after inner completion"))
			}
		}),
	}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{StreamModelResponses: true})
	events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
	failed := payloadsOf[runs.ModelCallFailed](events)
	if len(failed) != 1 || !reflect.DeepEqual(failed[0].KnownResponse, response) || failed[0].Accounting == nil || failed[0].FirstOutputLatencyMillis == nil || len(payloadsOf[runs.ModelCallCompleted](events)) != 0 {
		t.Fatalf("stream inner evidence lost or promoted: %+v", failed)
	}
}

func TestUnavailableModelStreamClosesAttemptWithoutInventingResponse(t *testing.T) {
	for _, test := range []struct {
		name     string
		sequence iter.Seq2[*chat.ResponseDelta, error]
	}{
		{name: "nil sequence"},
		{name: "empty sequence", sequence: func(func(*chat.ResponseDelta, error) bool) {}},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := struct {
				chat.Model
				chat.Streamer
			}{
				Model: chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
					t.Error("unexpected nonstreaming call")
					return nil, errors.New("unexpected nonstreaming call")
				}),
				Streamer: chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
					return test.sequence
				}),
			}
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{StreamModelResponses: true})
			events := runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
			failed := payloadsOf[runs.ModelCallFailed](events)
			if len(failed) != 1 || failed[0].Observation != (runs.ModelObservation{}) ||
				failed[0].FirstOutputLatencyMillis != nil || len(payloadsOf[runs.ModelCallCompleted](events)) != 0 {
				t.Fatalf("unavailable stream failed to close its model attempt: %+v", failed)
			}
			ends := payloadsOf[runs.SegmentEnded](events)
			if len(ends) != 1 || ends[0].Reason != run.OutcomeFailed || ends[0].Failure() == nil ||
				ends[0].Failure().Kind != run.FailureProviderUnavailable || len(ends[0].UnresolvedEffects()) != 1 {
				t.Fatalf("stream failure lost its provider classification or unknown Effect: %+v", ends)
			}
		})
	}
}

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
