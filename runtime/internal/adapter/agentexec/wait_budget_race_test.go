//go:build race

package agentexec

import "time"

func waitBudget(d time.Duration) time.Duration { return d * 12 }
