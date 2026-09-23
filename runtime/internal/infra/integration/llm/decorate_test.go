package llm

import (
	"context"
	"iter"
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

type streamerFunc func(context.Context, *chat.Request) iter.Seq2[*chat.ResponseDelta, error]

func (s streamerFunc) Stream(
	ctx context.Context,
	request *chat.Request,
) iter.Seq2[*chat.ResponseDelta, error] {
	return s(ctx, request)
}

// TestDecorateModelRepublishesTheInnerCapabilitySet is the rule every wrapper in
// this package inherits by construction. A provider publishes streaming and
// complete-request token counting as assertions on the same instance, so a
// wrapper that returned a plain chat.Model would silently turn a streaming
// provider into a non-streaming one, and the first symptom would be a Run that
// no longer shows tokens as they arrive.
func TestDecorateModelRepublishesTheInnerCapabilitySet(t *testing.T) {
	streaming := struct {
		chat.Model
		streamerFunc
	}{Model: callOnlyModel{}, streamerFunc: func(context.Context, *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
		return nil
	}}
	counting := struct {
		chat.Model
		inputTokenCounterFunc
	}{Model: callOnlyModel{}, inputTokenCounterFunc: func(context.Context, *chat.Request) (int64, error) {
		return 7, nil
	}}
	both := struct {
		chat.Model
		streamerFunc
		inputTokenCounterFunc
	}{
		Model:        callOnlyModel{},
		streamerFunc: streaming.streamerFunc,

		inputTokenCounterFunc: counting.inputTokenCounterFunc,
	}

	for name, test := range map[string]struct {
		inner  chat.Model
		stream bool
		count  bool
	}{
		"call only":          {inner: callOnlyModel{}},
		"streaming":          {inner: streaming, stream: true},
		"counting":           {inner: counting, count: true},
		"streaming counting": {inner: both, stream: true, count: true},
	} {
		t.Run(name, func(t *testing.T) {
			decorated, err := decorateModel(test.inner, modelDecoration{
				name: "test decoration",
				call: func(inner chat.Model) chat.Model { return inner },
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, streams := decorated.(chat.Streamer); streams != test.stream {
				t.Errorf("streaming capability = %t, want %t", streams, test.stream)
			}
			if _, counts := decorated.(InputTokenCounter); counts != test.count {
				t.Errorf("token counting capability = %t, want %t", counts, test.count)
			}
		})
	}
}

// TestDecorateModelRefusesADecorationThatDropsTheModel keeps a wrapper from
// publishing a half-built instance: a decoration that returns nothing is a
// construction failure, not a model that fails on first use.
func TestDecorateModelRefusesADecorationThatDropsTheModel(t *testing.T) {
	if _, err := decorateModel(callOnlyModel{}, modelDecoration{
		name: "empty decoration",
		call: func(chat.Model) chat.Model { return nil },
	}); err == nil {
		t.Fatal("decorateModel accepted a decoration that returned no model")
	}
	if _, err := decorateModel(nil, modelDecoration{
		name: "nil model",
		call: func(inner chat.Model) chat.Model { return inner },
	}); err == nil {
		t.Fatal("decorateModel accepted a nil model")
	}
}
