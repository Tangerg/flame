package exec

import (
	"errors"
	"testing"
	"time"
)

func TestTimeoutDistinguishesDisabledFromPositiveDuration(t *testing.T) {
	t.Parallel()

	if duration, enabled := (Timeout{}).Duration(); enabled || duration != 0 {
		t.Fatalf("zero timeout = (%s,%t), want disabled", duration, enabled)
	}
	timeout, err := NewTimeout(250 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if duration, enabled := timeout.Duration(); !enabled || duration != 250*time.Millisecond {
		t.Fatalf("timeout = (%s,%t), want (250ms,true)", duration, enabled)
	}
	for _, duration := range []time.Duration{0, -time.Millisecond} {
		if _, err := NewTimeout(duration); !errors.Is(err, ErrInvalidTimeout) {
			t.Fatalf("NewTimeout(%s) error = %v, want ErrInvalidTimeout", duration, err)
		}
	}
}

func testTimeout(t *testing.T, duration time.Duration) Timeout {
	t.Helper()
	timeout, err := NewTimeout(duration)
	if err != nil {
		t.Fatal(err)
	}
	return timeout
}
