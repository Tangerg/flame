package bootstrap

import (
	"testing"
	"time"
)

func TestShutdownWaitPolicyRequiresAnExplicitPositiveBudget(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		if _, err := newShutdownWaitPolicy(timeout); err == nil {
			t.Fatalf("newShutdownWaitPolicy(%v) accepted", timeout)
		}
	}
	if _, err := shutdownWaitTimeout(shutdownWaitPolicy{}); err == nil {
		t.Fatal("unconfigured shutdown wait policy returned a timeout")
	}
}
