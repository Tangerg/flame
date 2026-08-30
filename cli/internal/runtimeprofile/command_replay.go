package runtimeprofile

import (
	"time"

	"github.com/Tangerg/flame/cli/internal/commandreplay"
)

// CommandReplayPolicy projects an optional discovered Runtime profile into
// one explicit replay policy. A missing profile is unavailable; a present but
// invalid advertised capability is an error and never degrades to unavailable.
func CommandReplayPolicy(profile *Profile) (commandreplay.Policy, error) {
	return CommandReplayPolicyWithClock(profile, time.Now)
}

// CommandReplayPolicyWithClock is the deterministic form used at exact
// deadline boundaries and by long-running admission tests.
func CommandReplayPolicyWithClock(
	profile *Profile,
	now func() time.Time,
) (commandreplay.Policy, error) {
	if profile == nil {
		return commandreplay.UnavailablePolicyWithClock(now)
	}
	return commandreplay.NewPolicyWithClock(profile.Limits.CommandReplay, now)
}
