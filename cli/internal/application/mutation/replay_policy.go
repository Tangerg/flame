package mutation

import (
	"errors"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
)

// ReplayPolicy joins the connected Runtime's replay capability with the clock
// used to evaluate it. Discovery always advertises a replay store, so a command
// is admitted only while that store still promises its outcome.
type ReplayPolicy struct {
	capability replay.Capability
	now        func() time.Time
}

func NewReplayPolicy(capability replay.Capability, now func() time.Time) (ReplayPolicy, error) {
	policy := ReplayPolicy{capability: capability, now: now}
	if err := policy.validate(); err != nil {
		return ReplayPolicy{}, err
	}
	return policy, nil
}

func (p ReplayPolicy) validate() error {
	if p.now == nil {
		return errors.New("command replay policy clock is nil")
	}
	return p.capability.Validate()
}

func (p ReplayPolicy) Now() time.Time { return p.now().UTC() }

func (p ReplayPolicy) NewGuard() (replay.Guard, error) {
	return p.NewGuardAt(p.Now())
}

func (p ReplayPolicy) NewGuardAt(stagedAt time.Time) (replay.Guard, error) {
	until, err := p.capability.Deadline(stagedAt)
	if err != nil {
		return replay.Guard{}, err
	}
	return replay.NewProtectedGuard(p.capability.Namespace(), until)
}

func (p ReplayPolicy) SameStore(guard replay.Guard) bool {
	if guard.Validate() != nil {
		return false
	}
	return guard.Protected() &&
		guard.Namespace() == p.capability.Namespace()
}

func (p ReplayPolicy) Replayable(guard replay.Guard) bool {
	return p.SameStore(guard) && p.Now().Before(guard.Until())
}
