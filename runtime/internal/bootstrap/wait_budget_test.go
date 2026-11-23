package bootstrap

import (
	"testing"
	"time"
)

func lifecycleWaitBudget(t *testing.T) time.Duration {
	t.Helper()
	deadline, ok := t.Deadline()
	if !ok {
		return time.Minute
	}
	return max(5*time.Second, time.Until(deadline)/4)
}
