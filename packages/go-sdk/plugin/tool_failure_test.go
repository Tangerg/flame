package plugin

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestDefiniteToolFailurePreservesCompleteOutcome(t *testing.T) {
	for _, kind := range []toolcontract.FailureKind{toolcontract.FailureKindFailed, toolcontract.FailureKindRejected} {
		t.Run(string(kind), func(t *testing.T) {
			failure, err := toolcontract.NewFailure(toolcontract.FailureConfig{
				Kind: kind, Cause: context.DeadlineExceeded, Output: chat.NewTextToolOutput("original public result"),
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, original := range []error{failure, fmt.Errorf("adapter: %w", failure)} {
				projected := DefiniteToolFailure(original)
				if projected != original {
					t.Fatalf("complete outcome wrapped again: %v", projected)
				}
				if got, known := toolcontract.ClassifyFailure(projected); !known || got != failure {
					t.Fatalf("original outcome changed: %v", projected)
				}
			}
		})
	}
}

func TestDefiniteToolFailureRetainsRawCauseAndExecutionCancellation(t *testing.T) {
	cause := errors.New("argument was invalid")
	failure, known := toolcontract.ClassifyFailure(DefiniteToolFailure(cause))
	if !known || failure.Kind() != toolcontract.FailureKindFailed || failure.Cause() != cause {
		t.Fatalf("raw failure projection: %v", failure)
	}
	for _, original := range []error{nil, context.Canceled, context.DeadlineExceeded, errors.Join(context.Canceled, cause)} {
		if got := DefiniteToolFailure(original); got != original {
			t.Fatalf("execution cancellation changed: %v", got)
		}
	}
}
