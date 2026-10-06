package runtimebinding

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
)

func TestNegotiatedReplayPolicyKeepsUnavailableAndInvalidDistinct(t *testing.T) {
	t.Parallel()

	unavailable, err := mutation.PolicyFromProfile(nil, time.Now)
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
	if _, err := mutation.PolicyFromProfile(&Profile{}, time.Now); err == nil {
		t.Fatal("invalid advertised command replay capability degraded to unavailable")
	}
}

func TestNegotiatedReplayPolicyProjectsTheAdvertisedStoreAndClock(t *testing.T) {
	t.Parallel()

	capability, err := replay.NewCapability(compatibleReplayNamespace, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	profile := new(profileWithReplayNamespace(t, capability.Namespace()))
	policy, err := mutation.PolicyFromProfile(profile, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	guard, err := policy.NewGuard()
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Available() || guard.Namespace() != capability.Namespace() ||
		!guard.Until().Equal(now.Add(10*time.Minute)) {
		t.Fatalf("advertised policy = %+v, guard %+v", policy, guard)
	}
}
