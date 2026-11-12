package pluginservices

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/core/invocation"
)

func TestProviderRetryHintsCannotCreateInvalidRunFailures(t *testing.T) {
	for _, kind := range (modelprovider.FailureKind("")).JSONSchemaEnum() {
		t.Run(kind, func(t *testing.T) {
			for _, hint := range []int64{0, 1, math.MaxInt64} {
				reported := modelprovider.Failure{Kind: modelprovider.FailureKind(kind), Execution: invocation.FailureNotExecuted, RetryAfterMillis: hint}
				if err := reported.Validate(); err != nil {
					t.Fatal(err)
				}
				translated := providerFailure(&reported)
				failure, ok := errors.AsType[*run.FailureError](translated)
				if !ok {
					t.Fatalf("lost classified failure: %v", translated)
				}
				if err := (run.Failure{Kind: failure.Kind, RetryAfter: failure.RetryAfter}).Validate(); err != nil {
					t.Fatalf("%s with hint %d produced an invalid Run failure: %v", kind, hint, err)
				}
				var expected time.Duration
				if failure.Kind.AllowsRetryAfter() {
					expected = time.Duration(min(hint, run.MaximumRetryAfter.Milliseconds())) * time.Millisecond
				}
				if failure.RetryAfter != expected || reported.RetryAfterMillis != hint {
					t.Fatalf("retry policy changed its source or delay: reported=%d projected=%s want=%s", reported.RetryAfterMillis, failure.RetryAfter, expected)
				}
				if invocation.ClassifyFailure(translated) != reported.Execution {
					t.Fatal("retry policy replaced the original execution evidence")
				}
			}
		})
	}
}
