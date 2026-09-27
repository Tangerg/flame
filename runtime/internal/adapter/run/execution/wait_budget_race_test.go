//go:build race

package execution

import "time"

func waitBudget(d time.Duration) time.Duration { return d * 12 }
