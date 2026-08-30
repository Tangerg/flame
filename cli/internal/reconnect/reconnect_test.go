package reconnect

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/agent"
)

func TestReconnectRetriesOnlyTransientErrorsWithinBudget(t *testing.T) {
	policy, err := newPolicy(3, 10*time.Millisecond, 25*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	for failure, want := range []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 25 * time.Millisecond} {
		got, ok, err := policy.Next(failure+1, agent.ErrDisconnected)
		if err != nil || !ok || got != want {
			t.Fatalf("failure %d = %s, %v; want %s", failure+1, got, ok, want)
		}
	}
	if _, ok, err := policy.Next(4, agent.ErrDisconnected); err != nil || ok {
		t.Fatal("retry budget was exceeded")
	}
	if _, ok, err := policy.Next(1, agent.ErrEventConflict); err != nil || ok {
		t.Fatal("identity conflict was treated as transient")
	}
	if _, ok, err := policy.Next(1, agent.ErrReplayUnavailable); err != nil || ok {
		t.Fatal("unavailable replay was treated as a retryable disconnect")
	}
}

func TestCommandCommitRetriesHonorTheRuntimeBackoffFloor(t *testing.T) {
	policy, err := newPolicy(2, 10*time.Millisecond, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if delay, ok, err := policy.Next(1, agent.ErrCommandInProgress); err != nil || !ok || delay != time.Second {
		t.Fatalf("command progress retry = %s, %t; want 1s, true", delay, ok)
	}
	if !Retryable(agent.ErrCommandInProgress) {
		t.Fatal("command progress was not retryable")
	}
	if Retryable(agent.ErrCommandConflict) {
		t.Fatal("command identity conflict was retryable")
	}
}

func TestReconnectPolicyRequiresNamedDisabledOrConfiguredState(t *testing.T) {
	t.Parallel()
	if _, retryable, err := Disabled().Next(1, agent.ErrDisconnected); err != nil || retryable {
		t.Fatalf("disabled Next = (%t, %v)", retryable, err)
	}
	if _, _, err := (Policy{}).Next(1, agent.ErrDisconnected); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("zero policy Next = %v", err)
	}
	if _, err := New(-1); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("New(-1) = %v", err)
	}
}

func TestRetryableRecognizesOnlyClassifiedDisconnects(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "disconnect", err: agent.ErrDisconnected, want: true},
		{name: "wrapped disconnect", err: fmt.Errorf("transport closed: %w", agent.ErrDisconnected), want: true},
		{name: "business error", err: errors.New("server not found")},
		{name: "compatibility error", err: agent.ErrIncompatibleRuntime},
		{name: "cancellation", err: context.Canceled},
		{name: "nil"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Retryable(test.err); got != test.want {
				t.Fatalf("Retryable(%v) = %t, want %t", test.err, got, test.want)
			}
		})
	}
}
