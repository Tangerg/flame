package run

import (
	"errors"
	"fmt"
	"time"
)

// MaximumRetryAfter is the largest retry delay that survives the domain's
// whole-second persistence and protocol representations without overflow.
const MaximumRetryAfter = time.Duration((1<<63-1)/int64(time.Second)) * time.Second

// FailureKind classifies a Run failure without depending on provider
// error text. Integrations translate concrete failures at their boundary;
// durable Run records retain this stable vocabulary.
type FailureKind string

const (
	FailureInternal            FailureKind = "internal"
	FailureLost                FailureKind = "lost"
	FailureAgentStuck          FailureKind = "agent_stuck"
	FailureRateLimited         FailureKind = "rate_limited"
	FailureInvalidCredentials  FailureKind = "invalid_credentials"
	FailureTimeout             FailureKind = "timeout"
	FailureProviderUnavailable FailureKind = "provider_unavailable"
	FailureProviderRejected    FailureKind = "provider_rejected"
)

// Valid reports whether f is part of the durable provider-neutral taxonomy.
func (f FailureKind) Valid() bool {
	switch f {
	case FailureInternal, FailureLost, FailureAgentStuck, FailureRateLimited,
		FailureInvalidCredentials, FailureTimeout, FailureProviderUnavailable,
		FailureProviderRejected:
		return true
	default:
		return false
	}
}

// String names the kind for diagnostics — parity with the package's other
// enums (State / Outcome), so a FailureError without an error chain reports a
// legible name instead of a raw integer.
func (f FailureKind) String() string {
	if !f.Valid() {
		return "unknown"
	}
	return string(f)
}

// AllowsRetryAfter reports whether waiting can plausibly clear this failure.
// Other kinds require a different recovery action and cannot carry a delay.
func (f FailureKind) AllowsRetryAfter() bool {
	switch f {
	case FailureRateLimited, FailureTimeout, FailureProviderUnavailable:
		return true
	default:
		return false
	}
}

// Failure is the durable, provider-neutral explanation for a failed Run.
type Failure struct {
	Kind       FailureKind
	Detail     string
	DocURL     string
	RetryAfter time.Duration
}

// Validate reports whether the failure carries a known kind and a meaningful
// retry delay only for retryable classifications.
func (f Failure) Validate() error {
	if !f.Kind.Valid() {
		return fmt.Errorf("run: unknown failure kind %q", f.Kind)
	}
	if f.RetryAfter < 0 {
		return fmt.Errorf("run: failure retry delay must not be negative")
	}
	if f.RetryAfter > MaximumRetryAfter {
		return fmt.Errorf("run: failure retry delay exceeds the representable whole-second range")
	}
	if f.RetryAfter > 0 && !f.Kind.AllowsRetryAfter() {
		return fmt.Errorf("run: failure kind %s cannot carry a retry delay", f.Kind)
	}
	return nil
}

// RetryAfterSeconds returns the shortest whole-second delay that does not
// shorten the provider's retry hint. Durable and wire representations use
// seconds, while provider protocols may supply finer-grained delays.
func (f Failure) RetryAfterSeconds() int {
	if f.RetryAfter <= 0 {
		return 0
	}
	if f.RetryAfter > MaximumRetryAfter {
		return int(MaximumRetryAfter / time.Second)
	}
	seconds := f.RetryAfter / time.Second
	if f.RetryAfter%time.Second != 0 {
		seconds++
	}
	return int(seconds)
}

// RetryAfterFromSeconds restores a whole-second retry delay without allowing
// persistence or protocol input to overflow time.Duration.
func RetryAfterFromSeconds(seconds int) (time.Duration, error) {
	if seconds < 0 {
		return 0, errors.New("run: retry-after seconds must not be negative")
	}
	if int64(seconds) > int64(MaximumRetryAfter/time.Second) {
		return 0, errors.New("run: retry-after seconds exceed the representable duration")
	}
	return time.Duration(seconds) * time.Second, nil
}

// FailureError carries a typed Run classification while preserving the
// original error chain for diagnostics. RetryAfter is meaningful only for
// retryable kinds and may be zero when the provider supplied no hint.
type FailureError struct {
	Kind       FailureKind
	RetryAfter time.Duration
	Err        error
}

func (f *FailureError) Error() string {
	if f == nil {
		return "run failure"
	}
	if f.Err != nil {
		return f.Err.Error()
	}
	return "run failure: " + f.Kind.String()
}

func (f *FailureError) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.Err
}
