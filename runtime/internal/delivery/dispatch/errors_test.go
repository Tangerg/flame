package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestCancellationProjectionPreservesSafeWireProblem(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("unknown failure")} {
		t.Run(cause.Error(), func(t *testing.T) {
			failure := delivery.ProjectError(fmt.Errorf("private endpoint credential: %w", cause))
			if !errors.Is(failure, protocol.ErrInternalError) {
				t.Fatalf("failure = %v, want internal_error", failure)
			}
			if strings.Contains(failure.Error(), "private") || strings.Contains(errors.Unwrap(failure).Error(), "private") {
				t.Fatalf("failure retained private endpoint details: %v", failure)
			}
			rpcErr := errorToRPC(failure)
			if rpcErr.Code != codeInternalError || rpcErr.Message != protocol.ProblemInternalError {
				t.Fatalf("RPC error = %+v, want internal_error", rpcErr)
			}
			var problem protocol.ProblemData
			if err := json.Unmarshal(rpcErr.Data, &problem); err != nil {
				t.Fatal(err)
			}
			if problem.Type != protocol.ProblemInternalError || problem.Detail != "the runtime could not complete the request" {
				t.Fatalf("wire problem = %+v, want safe internal_error", problem)
			}
		})
	}
}

// The symbol says the first execution is still running; the backoff says how
// long to wait. A separate retryable flag said neither and was redundant with
// both — the wire omits false, so a client could never tell "not retryable" from
// "unclassified" anyway.
func TestIdempotencyInProgressErrorCarriesItsBackoff(t *testing.T) {
	rpcErr := errorToRPC(fmt.Errorf("%w: first execution has not completed", protocol.ErrIdempotencyInProgress))
	if rpcErr.Code != codeIdempotencyInProgress {
		t.Fatalf("code = %d, want %d", rpcErr.Code, codeIdempotencyInProgress)
	}
	var problem protocol.ProblemData
	if err := json.Unmarshal(rpcErr.Data, &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Type != protocol.ErrIdempotencyInProgress.Error() || problem.RetryAfterSeconds != 1 {
		t.Fatalf("problem = %+v", problem)
	}
}

func TestMalformedStructuredProblemFailsClosedAsInternalError(t *testing.T) {
	t.Parallel()

	// capability_not_negotiated without its non-empty requirements is not a legal
	// frame. The error boundary must not publish a payload whose recovery contract
	// it cannot satisfy.
	rpcErr := problemError(
		protocol.ErrCapabilityNotNeg,
		"missing structured requirements",
	)
	if rpcErr.Code != codeInternalError || rpcErr.Message != protocol.ProblemInternalError {
		t.Fatalf("fallback error = %+v, want internal_error", rpcErr)
	}
	var problem protocol.ProblemData
	if err := json.Unmarshal(rpcErr.Data, &problem); err != nil {
		t.Fatalf("decode fallback problem: %v", err)
	}
	if problem.Type != protocol.ProblemInternalError {
		t.Fatalf("fallback problem type = %q, want %q", problem.Type, protocol.ProblemInternalError)
	}
}
