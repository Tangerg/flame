package retry

import (
	"context"
	"errors"
	"fmt"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"math"
	"testing"
	"time"
)

func TestReconnectContinuesWithBoundedDelay(t *testing.T) {
	for _, failure := range []int{1, 2, 20, 1000, math.MaxInt} {
		delay, again, err := ReconnectDelay(failure, agent.ErrDisconnected)
		if err != nil || !again || delay < 50*time.Millisecond || delay > time.Second {
			t.Fatalf("retry %d: %v, %v, %v", failure, delay, again, err)
		}
	}
	for _, cause := range []error{agent.ErrEventConflict, agent.ErrReplayUnavailable, context.Canceled} {
		if _, again, err := ReconnectDelay(1, cause); err != nil || again {
			t.Fatalf("retry permanent error %v: %v, %v", cause, again, err)
		}
	}
	if _, _, err := ReconnectDelay(0, agent.ErrDisconnected); !errors.Is(err, ErrInvalidBackoff) {
		t.Fatalf("invalid count: %v", err)
	}
}

func TestCommandProgressUsesBackoffFloor(t *testing.T) {
	if delay, again, err := ReconnectDelay(1, agent.ErrCommandInProgress); err != nil || !again || delay != time.Second {
		t.Fatalf("progress delay: %v, %v, %v", delay, again, err)
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
