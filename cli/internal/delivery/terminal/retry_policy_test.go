package terminal

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/retry"
)

func testBackoff(t testing.TB, base, maximum time.Duration) retry.Backoff {
	t.Helper()
	backoff, err := retry.NewBackoff(base, maximum)
	if err != nil {
		t.Fatalf("retry.NewBackoff(%s, %s): %v", base, maximum, err)
	}
	return backoff
}

func runtimeRecoveryFirstDelay(t testing.TB) time.Duration {
	t.Helper()
	delay, err := runtimeRecoveryBackoff.Delay(1)
	if err != nil {
		t.Fatalf("runtime recovery backoff: %v", err)
	}
	return delay
}
