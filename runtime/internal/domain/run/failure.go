package run

import (
	"errors"
	"fmt"
	"time"
)

const MaximumRetryAfter = time.Duration((1<<63-1)/int64(time.Second)) * time.Second

type FailureKind string

const (
	FailureInternal            FailureKind = "internal_error"
	FailureAgentStuck          FailureKind = "agent_stuck"
	FailureRateLimited         FailureKind = "rate_limited"
	FailureInvalidCredentials  FailureKind = "invalid_credentials"
	FailureTimeout             FailureKind = "timeout"
	FailureProviderUnavailable FailureKind = "provider_unavailable"
	FailureProviderRejected    FailureKind = "provider_rejected"
)

func (f FailureKind) Valid() bool {
	switch f {
	case FailureInternal, FailureAgentStuck, FailureRateLimited,
		FailureInvalidCredentials, FailureTimeout, FailureProviderUnavailable,
		FailureProviderRejected:
		return true
	default:
		return false
	}
}

func (f FailureKind) AllowsRetryAfter() bool {
	switch f {
	case FailureRateLimited, FailureTimeout, FailureProviderUnavailable:
		return true
	default:
		return false
	}
}

type Failure struct {
	Kind       FailureKind
	Detail     string
	DocURL     string
	RetryAfter time.Duration
}

func (f Failure) Validate() error {
	if !f.Kind.Valid() {
		return fmt.Errorf("run: unknown failure kind %q", f.Kind)
	}
	if f.RetryAfter < 0 {
		return errors.New("run: failure retry delay must not be negative")
	}
	if f.RetryAfter > MaximumRetryAfter {
		return errors.New("run: failure retry delay exceeds the representable whole-second range")
	}
	if f.RetryAfter > 0 && !f.Kind.AllowsRetryAfter() {
		return fmt.Errorf("run: failure kind %s cannot carry a retry delay", f.Kind)
	}
	return nil
}

func (f Failure) RetryAfterSeconds() int {
	seconds := f.RetryAfter / time.Second
	if f.RetryAfter%time.Second != 0 {
		seconds++
	}
	return int(seconds)
}

func RetryAfterFromSeconds(seconds int) (time.Duration, error) {
	if seconds < 0 {
		return 0, errors.New("run: retry-after seconds must not be negative")
	}
	if int64(seconds) > int64(MaximumRetryAfter/time.Second) {
		return 0, errors.New("run: retry-after seconds exceed the representable duration")
	}
	return time.Duration(seconds) * time.Second, nil
}

type FailureError struct {
	Kind       FailureKind
	RetryAfter time.Duration
	Err        error
}

func (f *FailureError) Error() string {
	if f.Err != nil {
		return f.Err.Error()
	}
	return "run failure: " + string(f.Kind)
}

func (f *FailureError) Unwrap() error { return f.Err }
