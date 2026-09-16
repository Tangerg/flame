//go:build !race

package agentexec

import "time"

// waitBudget scales how long a test waits for the executor to reach a state.
// The race build multiplies it: these tests drive real goroutines, and the
// detector's instrumentation is the difference between a fast check and a
// spurious failure under load.
func waitBudget(d time.Duration) time.Duration { return d }
