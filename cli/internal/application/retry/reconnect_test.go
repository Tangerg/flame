package retry

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func TestReconnectRetriesOnlyTransientErrorsWithinBudget(t *testing.T) {
	policy, err := newReconnectPolicy(3, 10*time.Millisecond, 25*time.Millisecond)
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
	policy, err := newReconnectPolicy(2, 10*time.Millisecond, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if delay, ok, err := policy.Next(1, agent.ErrCommandInProgress); err != nil || !ok || delay != time.Second {
		t.Fatalf("command progress retry = %s, %t; want 1s, true", delay, ok)
	}
	if !IsReconnectable(agent.ErrCommandInProgress) {
		t.Fatal("command progress was not retryable")
	}
	if IsReconnectable(agent.ErrCommandConflict) {
		t.Fatal("command identity conflict was retryable")
	}
}

func TestReconnectPolicyRequiresNamedDisabledOrBoundedState(t *testing.T) {
	t.Parallel()
	if _, reconnectable, err := DisabledReconnectPolicy().Next(1, agent.ErrDisconnected); err != nil || reconnectable {
		t.Fatalf("disabled Next = (%t, %v)", reconnectable, err)
	}
	if _, _, err := (ReconnectPolicy{}).Next(1, agent.ErrDisconnected); !errors.Is(err, ErrInvalidReconnectPolicy) {
		t.Fatalf("zero policy Next = %v", err)
	}
	if _, err := NewReconnectPolicy(-1); !errors.Is(err, ErrInvalidReconnectPolicy) {
		t.Fatalf("NewReconnectPolicy(-1) = %v", err)
	}
	for _, bounds := range [][2]time.Duration{{0, time.Second}, {time.Second, time.Millisecond}} {
		if _, err := newReconnectPolicy(1, bounds[0], bounds[1]); !errors.Is(err, ErrInvalidReconnectPolicy) {
			t.Fatalf("newReconnectPolicy(1, %s, %s) = %v", bounds[0], bounds[1], err)
		}
	}
}

func TestIsReconnectableRecognizesOnlyClassifiedDisconnects(t *testing.T) {
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
			if got := IsReconnectable(test.err); got != test.want {
				t.Fatalf("IsReconnectable(%v) = %t, want %t", test.err, got, test.want)
			}
		})
	}
}
