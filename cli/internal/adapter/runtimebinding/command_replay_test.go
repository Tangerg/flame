package runtimebinding

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
)

func TestNegotiatedReplayPolicyRequiresANegotiatedProfile(t *testing.T) {
	t.Parallel()

	if _, err := mutation.PolicyFromProfile(nil, time.Now); err == nil {
		t.Fatal("a missing profile produced a replay policy")
	}
	if _, err := mutation.PolicyFromProfile(&Profile{}, time.Now); err == nil {
		t.Fatal("an unnegotiated profile produced a replay policy")
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
	if guard.Namespace() != capability.Namespace() ||
		!guard.Until().Equal(now.Add(10*time.Minute)) {
		t.Fatalf("advertised policy = %+v, guard %+v", policy, guard)
	}
}
