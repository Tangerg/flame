package mutation

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
)

func TestPolicyCreatesAndEvaluatesOneStoreBoundDeadline(t *testing.T) {
	t.Parallel()

	stagedAt := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	now := stagedAt
	capability, err := replay.NewCapability("runtime-a", 10*time.Minute)
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
	if !policy.SameStore(guard) || !policy.Replayable(guard) {
		t.Fatalf("fresh advertised guard was not replayable: %+v", guard)
	}
	other, err := replay.NewProtectedGuard("runtime-b", guard.Until())
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

func TestPolicyRefusesToEvaluateWithoutAClock(t *testing.T) {
	t.Parallel()

	capability, err := replay.NewCapability("runtime-a", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewReplayPolicy(capability, nil); err == nil {
		t.Fatal("policy was built without a clock")
	}
	if err := (ReplayPolicy{}).validate(); err == nil {
		t.Fatal("the zero policy was valid")
	}
}
