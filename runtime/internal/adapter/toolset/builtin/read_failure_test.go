package builtin

import (
	"context"
	"errors"
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type failingConversationSearch struct{ err error }

func (f failingConversationSearch) SearchTranscript(
	context.Context, string, int,
) ([]transcript.SearchHit, error) {
	return nil, f.err
}

// TestReadToolServiceFailureIsDefinite covers what a failed read must settle as.
// A search performs no external mutation, so its outcome is provable; an
// unclassified error instead tells the Host the durable effect is unknown, and
// the Host ends the root Run and every delegated child over a store that was
// only being read.
func TestReadToolServiceFailureIsDefinite(t *testing.T) {
	executable, err := NewConversationSearch(failingConversationSearch{
		err: errors.New("transcript index unavailable"),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, callErr := callTextTool(t.Context(), executable, `{"query":"deploy"}`)
	failure, classified := errors.AsType[*toolcontract.Failure](callErr)
	if !classified {
		t.Fatalf("search failure = %v, want a classified Tool failure", callErr)
	}
	if failure.Kind() != toolcontract.FailureKindFailed || failure.Validate() != nil {
		t.Fatalf("failure kind = %q, valid = %v", failure.Kind(), failure.Validate())
	}
	if !errors.Is(callErr, context.Canceled) && failure.Cause() == nil {
		t.Fatal("classified failure lost its cause")
	}
}

// TestReadToolCancellationStaysUnclassified is the other half: cancellation
// belongs to whoever owns the execution, never to the model as feedback.
func TestReadToolCancellationStaysUnclassified(t *testing.T) {
	executable, err := NewConversationSearch(failingConversationSearch{err: context.Canceled})
	if err != nil {
		t.Fatal(err)
	}
	_, callErr := callTextTool(t.Context(), executable, `{"query":"deploy"}`)
	if !errors.Is(callErr, context.Canceled) {
		t.Fatalf("cancellation = %v, want context.Canceled", callErr)
	}
	if _, classified := errors.AsType[*toolcontract.Failure](callErr); classified {
		t.Fatal("cancellation was turned into model feedback")
	}
}
