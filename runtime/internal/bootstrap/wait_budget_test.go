package bootstrap

import (
	"testing"
	"time"
)

// lifecycleWaitBudget bounds how long an end-to-end lifecycle assertion waits
// for a Runtime to reach the state it is proving.
//
// It is a stall detector, not a performance assertion: its job is to name the
// phase that hung instead of letting the package reach `go test`'s own timeout
// with no attribution. A hand-picked wall clock cannot do that job, because how
// long these tests take is decided by what else is running — the race
// detector's instrumentation, or the rest of the suite on the same machine — so
// a number large enough to be reliable under load is one that no longer fails
// fast, and a number that fails fast turns machine load into a test failure.
// The deadline the run was actually given is the only fact here that scales
// with how the suite was invoked.
func lifecycleWaitBudget(t *testing.T) time.Duration {
	t.Helper()
	deadline, ok := t.Deadline()
	if !ok {
		return time.Minute
	}
	return max(5*time.Second, time.Until(deadline)/4)
}
