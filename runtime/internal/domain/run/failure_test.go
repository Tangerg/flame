package run

import (
	"math"
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
			failure := Failure{RetryAfter: test.delay}
			if got := failure.RetryAfterSeconds(); got != test.want {
				t.Fatalf("RetryAfterSeconds() = %d, want %d", got, test.want)
			}
		})
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
	if err := (Failure{Kind: FailureRateLimited, RetryAfter: MaximumRetryAfter + time.Nanosecond}).Validate(); err == nil {
		t.Fatal("Failure.Validate accepted a delay that cannot round-trip through seconds")
	}
	if got := (Failure{RetryAfter: time.Duration(math.MaxInt64)}).RetryAfterSeconds(); got != maximumSeconds {
		t.Fatalf("RetryAfterSeconds(max duration) = %d, want %d", got, maximumSeconds)
	}
}
