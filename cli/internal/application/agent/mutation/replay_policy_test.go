package mutation

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
)

func TestPolicyCreatesAndEvaluatesOneStoreBoundDeadline(t *testing.T) {
	t.Parallel()

	stagedAt := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	now := stagedAt
	capability, err := commandreplay.NewCapability("runtime-a", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := NewReplayPolicy(capability, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	guard, err := policy.NewGuardAt(stagedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Available() || !policy.CanStart(guard) || !policy.SameStore(guard) || !policy.Replayable(guard) {
		t.Fatalf("fresh advertised guard was not replayable: %+v", guard)
	}
	other, err := commandreplay.NewProtectedGuard("runtime-b", guard.Until())
	if err != nil {
		t.Fatal(err)
	}
	if policy.SameStore(other) || policy.Replayable(other) {
		t.Fatal("another Runtime store owned the command guard")
	}
	now = guard.Until()
	if policy.Replayable(guard) || policy.CanStart(guard) {
		t.Fatal("command remained replayable at its exact retention deadline")
	}
}

func TestPolicyRefusesEitherShapeWithoutAClock(t *testing.T) {
	t.Parallel()

	capability, err := commandreplay.NewCapability("runtime-a", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// Every evaluation reads Now, and both constructors admit through Validate,
	// so this is the one place either shape can be refused a clock.
	if _, err := NewReplayPolicy(capability, nil); err == nil {
		t.Fatal("advertised policy was built without a clock")
	}
	if _, err := UnavailableReplayPolicy(nil); err == nil {
		t.Fatal("unavailable policy was built without a clock")
	}
	if err := (ReplayPolicy{}).Validate(); err == nil {
		t.Fatal("the zero policy was valid")
	}
}

func TestUnavailablePolicyIsExplicitAndOwnsOnlyUnprotectedGuards(t *testing.T) {
	t.Parallel()

	policy, err := UnavailableReplayPolicy(time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	guard, err := policy.NewGuard()
	if err != nil {
		t.Fatal(err)
	}
	if policy.Available() || guard.Protected() || !policy.CanStart(guard) || policy.SameStore(guard) || policy.Replayable(guard) {
		t.Fatalf("unavailable policy projection = policy %+v, guard %+v", policy, guard)
	}
	if err := (ReplayPolicy{}).Validate(); err == nil {
		t.Fatal("zero ReplayPolicy was valid")
	}
}
