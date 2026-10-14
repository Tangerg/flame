package model

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

type auxiliaryCountFunc func(context.Context, *chat.Request) (int64, error)

func (f auxiliaryCountFunc) CountInputTokens(ctx context.Context, request *chat.Request) (int64, error) {
	return f(ctx, request)
}

func capacityFixture(t *testing.T, selection modelref.Selection, counter InputTokenCounter) SelectedChat {
	t.Helper()
	resolved, err := NewResolvedChat(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		t.Fatal("admission executed provider")
		return nil, nil
	}), counter)
	if err != nil {
		t.Fatal(err)
	}
	capture, err := NewSelectedChat(selection, resolved)
	if err != nil {
		t.Fatal(err)
	}
	return capture
}
func capacityRequest(t *testing.T, output int64) *chat.Request {
	t.Helper()
	request, err := chat.NewRequest(chat.NewSystemMessage("instructions"), chat.NewUserMessage(chat.NewTextPart("question")))
	if err != nil {
		t.Fatal(err)
	}
	request.Options.MaxOutputTokens = new(output)
	return request
}

func TestAuxiliaryCapacityUsesExactProviderCountAndReservesPublishedOutput(t *testing.T) {
	selection := testsupport.MustModelSelection("openai", "gpt-5-pro")
	for _, tc := range []struct {
		name          string
		output, count int64
		want          bool
		wantCalls     int
	}{
		{"exact input ceiling", 64, 272000, true, 1}, {"input exceeded", 64, 272001, false, 1},
		{"output shares context", 272000, 128000, true, 1}, {"reserved context exceeded", 272000, 128001, false, 1},
		{"output exceeds published maximum", 272001, 0, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			capture := capacityFixture(t, selection, auxiliaryCountFunc(func(_ context.Context, request *chat.Request) (int64, error) {
				calls++
				if len(request.Messages) != 2 || request.Options.MaxOutputTokens == nil || *request.Options.MaxOutputTokens != tc.output {
					t.Error("counter lost complete request/output reservation")
				}
				request.Messages[0] = chat.NewSystemMessage("counter mutation")
				return tc.count, nil
			}))
			request := capacityRequest(t, tc.output)
			before := request.Clone()
			err := capture.AdmitRequest(t.Context(), request)
			if (err == nil) != tc.want || calls != tc.wantCalls {
				t.Fatalf("capacity admission=%v calls=%d", err, calls)
			}
			if !reflect.DeepEqual(request, before) {
				t.Fatal("counter modified admitted input")
			}
		})
	}
}

func TestAuxiliaryCapacityCounterFailureAndNegativeCountNeverUseLocalFallback(t *testing.T) {
	failure := errors.New("counter unavailable")
	for _, tc := range []struct {
		name  string
		count int64
		err   error
	}{{"failure", 0, failure}, {"negative", -1, nil}, {"zero", 0, nil}, {"cancellation", 0, context.Canceled}} {
		t.Run(tc.name, func(t *testing.T) {
			capture := capacityFixture(t, testsupport.MustModelSelection("openai", "gpt-5-pro"), auxiliaryCountFunc(func(context.Context, *chat.Request) (int64, error) { return tc.count, tc.err }))
			err := capture.AdmitRequest(t.Context(), capacityRequest(t, 64))
			if err == nil || (tc.err != nil && !errors.Is(err, tc.err)) {
				t.Fatalf("counter failure became apparent capacity: %v", err)
			}
		})
	}
}

func TestAuxiliaryCapacityFallbackAndUnknownCapacityPreserveTheirMeaning(t *testing.T) {
	request := capacityRequest(t, 64)
	request.Messages[1] = chat.NewUserMessage(chat.NewTextPart(strings.Repeat("x", 1100000)))
	capture := capacityFixture(t, testsupport.MustModelSelection("openai", "gpt-5-pro"), nil)
	if err := capture.AdmitRequest(t.Context(), request); err == nil {
		t.Fatal("complete local request exceeded known capacity but was admitted")
	}
	calls := 0
	unknown := capacityFixture(t, testsupport.MustModelSelection("openai-compatible", "private-model"), auxiliaryCountFunc(func(context.Context, *chat.Request) (int64, error) { calls++; return 0, errors.New("unneeded counter") }))
	if err := unknown.AdmitRequest(t.Context(), request); err != nil || calls != 0 {
		t.Fatalf("unknown capacity invented a ceiling: calls=%d %v", calls, err)
	}
}

func TestAuxiliaryWrappingPreservesCapacityCounterAndSelectedReasoning(t *testing.T) {
	selection, err := modelref.NewWithReasoningEffort("openai", "gpt-5-pro", "high")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	capture := capacityFixture(t, selection, auxiliaryCountFunc(func(context.Context, *chat.Request) (int64, error) { calls++; return 272001, nil }))
	wrapped, err := capture.WithModel(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) { return nil, nil }))
	if err != nil {
		t.Fatal(err)
	}
	request := capacityRequest(t, 64)
	prepared, err := wrapped.PrepareRequest(request)
	if err != nil || prepared.Options.ReasoningEffort != chat.ReasoningEffort("high") || request.Options.ReasoningEffort != "" || wrapped.Selection() != selection {
		t.Fatalf("captured model option changed: %+v %v", prepared, err)
	}
	if err := wrapped.AdmitRequest(t.Context(), prepared); err == nil || calls != 1 {
		t.Fatalf("wrapper discarded original counter/ceiling: calls=%d %v", calls, err)
	}
	request.Options.ReasoningEffort = chat.ReasoningEffort("low")
	if _, err := wrapped.PrepareRequest(request); err == nil {
		t.Fatal("request replaced selected reasoning")
	}
}
