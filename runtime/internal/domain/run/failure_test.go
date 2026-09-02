package run

import (
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
