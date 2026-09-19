package runtimebinding

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
)

func TestCommandReplayPolicyRequiresNegotiatedProfile(t *testing.T) {
	for _, profile := range []*Profile{nil, {}} {
		if _, err := CommandReplayPolicy(profile); err == nil {
			t.Fatal("command replay policy accepted an incomplete profile")
		}
	}
}

func TestCommandReplayPolicyProjectsTheAdvertisedStoreAndClock(t *testing.T) {
	t.Parallel()

	capability, err := commandreplay.NewCapability(compatibleReplayNamespace, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	profile := new(profileWithReplayNamespace(t, capability.Namespace()))
	policy, err := CommandReplayPolicyWithClock(profile, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	guard, err := policy.NewGuard()
	if err != nil {
		t.Fatal(err)
	}
	if guard.Namespace() != capability.Namespace() ||
		!guard.Until().Equal(now.Add(capability.Retention())) {
		t.Fatalf("advertised policy = %+v, guard %+v", policy, guard)
	}
}
