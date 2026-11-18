package main

import (
	"testing"
	"time"
)

func TestShellTimeoutKillsDescendantProcessGroup(t *testing.T) {
	started := time.Now()
	got := Shell{}.RunHookCommand(t.Context(), commandRequest{
		Command: `sleep 5 & wait`,
		Timeout: 40 * time.Millisecond,
	})
	if !got.TimedOut {
		t.Fatalf("TimedOut = false, err=%v", got.Err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timed-out hook retained a descendant process for %s", elapsed)
	}
}
