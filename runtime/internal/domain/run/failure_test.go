package run

import (
	"strings"
	"testing"
	"time"
)

func TestFailureRetryAfterSecondsNeverShortensProviderHint(t *testing.T) {
	tests := []struct {
		name  string
		delay time.Duration
		want  int
	}{
		{name: "absent", want: 0},
		{name: "subsecond", delay: time.Millisecond, want: 1},
		{name: "exact second", delay: time.Second, want: 1},
		{name: "fractional second", delay: time.Second + time.Nanosecond, want: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			failure := Failure{Kind: FailureRateLimited, RetryAfter: test.delay}
			// Rounding is only asked of delays the domain admits, so a fixture
			// Validate rejects would prove nothing about a reachable projection.
			if err := failure.Validate(); err != nil {
				t.Fatalf("fixture is not a legal Failure: %v", err)
			}
			if got := failure.RetryAfterSeconds(); got != test.want {
				t.Fatalf("RetryAfterSeconds() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestValidateNamesTheFailureKindItRejected(t *testing.T) {
	// A String() that renders an invalid kind as a fixed word is invisible at the
	// call site: %q and %s prefer it, so the diagnostic loses the only value it
	// was written to report.
	err := Failure{Kind: FailureKind("teapot")}.Validate()
	if err == nil || !strings.Contains(err.Error(), "teapot") {
		t.Fatalf("Validate() = %v, want the rejected kind named", err)
	}
}

func TestRetryAfterWholeSecondRepresentationIsClosed(t *testing.T) {
	maximumSeconds := int(MaximumRetryAfter / time.Second)
	delay, err := RetryAfterFromSeconds(maximumSeconds)
	if err != nil || delay != MaximumRetryAfter {
		t.Fatalf("RetryAfterFromSeconds(maximum) = %v, %v", delay, err)
	}
	if _, err := RetryAfterFromSeconds(maximumSeconds + 1); err == nil {
		t.Fatal("RetryAfterFromSeconds accepted an overflowing delay")
	}
	if _, err := RetryAfterFromSeconds(-1); err == nil {
		t.Fatal("RetryAfterFromSeconds accepted a negative delay")
	}
	if err := (Failure{Kind: FailureRateLimited, RetryAfter: MaximumRetryAfter + time.Nanosecond}).Validate(); err == nil {
		t.Fatal("Failure.Validate accepted a delay that cannot round-trip through seconds")
	}
	// The largest delay the domain admits is the one rounding must still carry
	// whole, since nothing beyond it can reach a projection.
	widest := Failure{Kind: FailureRateLimited, RetryAfter: MaximumRetryAfter}
	if err := widest.Validate(); err != nil {
		t.Fatalf("widest legal delay was rejected: %v", err)
	}
	if got := widest.RetryAfterSeconds(); got != maximumSeconds {
		t.Fatalf("RetryAfterSeconds(widest legal delay) = %d, want %d", got, maximumSeconds)
	}
}
