package protocol_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestProjectedFailureRetainsIndependentCancellation(t *testing.T) {
	for _, cancellation := range []error{context.Canceled, context.DeadlineExceeded} {
		for _, original := range []error{protocol.NewFailure(protocol.ErrSessionNotFound, "session is absent"), fmt.Errorf("lookup: %w", protocol.ErrSessionNotFound)} {
			projected := protocol.ProjectError(errors.Join(original, cancellation))
			if !errors.Is(projected, cancellation) || !errors.Is(projected, protocol.ErrSessionNotFound) {
				t.Errorf("projection lost independent causes: %v", projected)
			}
			if projected.Problem().Type != protocol.ErrSessionNotFound.Error() {
				t.Errorf("problem = %+v", projected.Problem())
			}
		}
	}
}

func TestFailureKindDoesNotComeFromDiagnosticCauses(t *testing.T) {
	for _, kind := range []error{protocol.ErrInternalError, protocol.ErrInvalidParams, protocol.ErrIdempotencyConflict} {
		diagnostic := protocol.NewFailure(protocol.ErrInvalidRequest, "nested diagnostic")
		failure := protocol.NewFailure(kind, "boundary-owned explanation", diagnostic)
		if failure.Problem().Type != kind.Error() || failure.Problem().Detail != "boundary-owned explanation" || !errors.Is(failure, diagnostic) {
			t.Fatalf("diagnostic replaced its owner: %+v, cause=%v", failure.Problem(), failure.Unwrap())
		}
	}
}

func TestFailureKindRequiresAnExactRegisteredSentinel(t *testing.T) {
	for _, kind := range []error{fmt.Errorf("wrapped: %w", protocol.ErrInvalidParams), errors.Join(protocol.ErrInvalidParams, errors.New("private diagnostic"))} {
		failure := protocol.NewFailure(kind, "unsafe implicit classification")
		if failure.Problem().Type != protocol.ProblemInternalError || strings.Contains(failure.Error(), "private diagnostic") {
			t.Fatalf("an implicit classifier escaped the boundary: %+v", failure.Problem())
		}
	}
}
