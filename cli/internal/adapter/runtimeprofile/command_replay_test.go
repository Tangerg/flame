package runtimeprofile

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
)

func TestCommandReplayPolicyKeepsUnavailableAndInvalidDistinct(t *testing.T) {
	t.Parallel()

	unavailable, err := CommandReplayPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := unavailable.NewGuard()
	if err != nil {
		t.Fatal(err)
	}
	if unavailable.Available() || !unavailable.CanStart(guard) || unavailable.Replayable(guard) {
		t.Fatalf("unavailable policy = %+v, guard %+v", unavailable, guard)
	}
	if _, err := CommandReplayPolicy(&Profile{}); err == nil {
		t.Fatal("invalid advertised command replay capability degraded to unavailable")
	}
}

func TestCommandReplayPolicyProjectsTheAdvertisedStoreAndClock(t *testing.T) {
	t.Parallel()

	capability, err := commandreplay.NewCapability("runtime-a", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	profile := &Profile{Limits: Limits{CommandReplay: capability}}
	policy, err := CommandReplayPolicyWithClock(profile, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	guard, err := policy.NewGuard()
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Available() || guard.Namespace() != capability.Namespace() ||
		!guard.Until().Equal(now.Add(capability.Retention())) {
		t.Fatalf("advertised policy = %+v, guard %+v", policy, guard)
	}
}
