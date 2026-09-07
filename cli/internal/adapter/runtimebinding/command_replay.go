package runtimebinding

import (
	"errors"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
)

// CommandReplayPolicy projects the negotiated Runtime profile into the replay
// policy used by CLI mutations. A complete profile is required.
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
		return commandreplay.Policy{}, errors.New("command replay requires a runtime profile")
	}
	if err := profile.Validate(); err != nil {
		return commandreplay.Policy{}, err
	}
	limits := profile.discovery.Capabilities.Limits.Idempotency
	capability, err := commandreplay.NewCapability(limits.Namespace, time.Duration(limits.RetentionSeconds)*time.Second)
	if err != nil {
		return commandreplay.Policy{}, err
	}
	return commandreplay.NewPolicyWithClock(capability, now)
}
