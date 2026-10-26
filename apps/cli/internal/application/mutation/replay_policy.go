package mutation

import (
	"errors"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
)

// ReplayPolicy joins the connected Runtime's replay capability with the clock
// used to evaluate it. Discovery always advertises a replay store, so a command
// is admitted only while that store still promises its outcome.
type ReplayPolicy struct {
	capability authoring.ReplayCapability
	now        func() time.Time
}

func newReplayPolicy(capability authoring.ReplayCapability, now func() time.Time) (ReplayPolicy, error) {
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

func (p ReplayPolicy) NewGuard() (authoring.ReplayGuard, error) {
	return p.NewGuardAt(p.Now())
}

func (p ReplayPolicy) NewGuardAt(stagedAt time.Time) (authoring.ReplayGuard, error) {
	until, err := p.capability.Deadline(stagedAt)
	if err != nil {
		return authoring.ReplayGuard{}, err
	}
	return authoring.NewProtectedGuard(p.capability.Namespace(), until)
}

func (p ReplayPolicy) SameStore(guard authoring.ReplayGuard) bool {
	if guard.Validate() != nil {
		return false
	}
	return guard.Protected() &&
		guard.Namespace() == p.capability.Namespace()
}

func (p ReplayPolicy) Replayable(guard authoring.ReplayGuard) bool {
	return p.SameStore(guard) && p.Now().Before(guard.Until())
}
