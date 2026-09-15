//go:build !race

package bootstrap

import "time"

// lifecycleWaitBudget bounds how long an end-to-end lifecycle assertion waits
// for a Runtime to reach the state it is proving. The race build multiplies it:
// these tests drive the real engine, and the detector's instrumentation is the
// difference between a fast check and a spurious failure.
const lifecycleWaitBudget = 5 * time.Second
