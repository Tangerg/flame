package commandreplay

import (
	"testing"
	"time"
)

func TestPolicyCreatesAndEvaluatesOneStoreBoundDeadline(t *testing.T) {
	t.Parallel()

	stagedAt := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	now := stagedAt
	capability, err := NewCapability("runtime-a", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := NewPolicyWithClock(capability, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	guard, err := policy.NewGuardAt(stagedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.SameStore(guard) || !policy.Replayable(guard) {
		t.Fatalf("fresh advertised guard was not replayable: %+v", guard)
	}
	other, err := NewProtectedGuard("runtime-b", guard.Until())
	if err != nil {
		t.Fatal(err)
	}
	if policy.SameStore(other) || policy.Replayable(other) {
		t.Fatal("another Runtime store owned the command guard")
	}
	now = guard.Until()
	if policy.Replayable(guard) {
		t.Fatal("command remained replayable at its exact retention deadline")
	}
}

func TestPolicyRejectsMissingCapabilityAndClock(t *testing.T) {
	if err := (Policy{}).Validate(); err == nil {
		t.Fatal("zero Policy was valid")
	}
	if _, err := (Policy{}).NewGuard(); err == nil {
		t.Fatal("zero Policy created a guard")
	}
	if _, err := NewPolicyWithClock(Capability{}, time.Now); err == nil {
		t.Fatal("policy accepted a missing capability")
	}
	capability, err := NewCapability("runtime-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewPolicyWithClock(capability, nil); err == nil {
		t.Fatal("policy accepted a missing clock")
	}
}
