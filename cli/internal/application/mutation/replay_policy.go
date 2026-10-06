package mutation

import (
	"errors"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
)

type policyKind uint8

const (
	policyUnavailable policyKind = iota + 1
	policyAdvertised
)

// ReplayPolicy joins the currently connected Runtime replay capability with the
// clock used to evaluate it. Unavailable is explicit and never reconstructed
// from empty capability fields.
type ReplayPolicy struct {
	kind       policyKind
	capability replay.Capability
	now        func() time.Time
}

func NewReplayPolicy(capability replay.Capability, now func() time.Time) (ReplayPolicy, error) {
	return newReplayPolicy(ReplayPolicy{kind: policyAdvertised, capability: capability, now: now})
}

func UnavailableReplayPolicy(now func() time.Time) (ReplayPolicy, error) {
	return newReplayPolicy(ReplayPolicy{kind: policyUnavailable, now: now})
}

// newReplayPolicy admits a built policy through the rule the value already
// carries, so a constructor cannot accept something Validate would refuse.
func newReplayPolicy(policy ReplayPolicy) (ReplayPolicy, error) {
	if err := policy.validate(); err != nil {
		return ReplayPolicy{}, err
	}
	return policy, nil
}

func (p ReplayPolicy) validate() error {
	if p.now == nil {
		return errors.New("command replay policy clock is nil")
	}
	switch p.kind {
	case policyUnavailable:
		if p.capability != (replay.Capability{}) {
			return errors.New("unavailable command replay policy carries a capability")
		}
		return nil
	case policyAdvertised:
		return p.capability.Validate()
	default:
		return errors.New("command replay policy is not configured")
	}
}

func (p ReplayPolicy) Available() bool { return p.kind == policyAdvertised }

func (p ReplayPolicy) Now() time.Time { return p.now().UTC() }

func (p ReplayPolicy) NewGuard() (replay.Guard, error) {
	return p.NewGuardAt(p.Now())
}

func (p ReplayPolicy) NewGuardAt(stagedAt time.Time) (replay.Guard, error) {
	if p.kind == policyUnavailable {
		return replay.UnprotectedGuard(), nil
	}
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
	return p.kind == policyAdvertised && guard.Protected() &&
		guard.Namespace() == p.capability.Namespace()
}

func (p ReplayPolicy) Replayable(guard replay.Guard) bool {
	return p.SameStore(guard) && p.Now().Before(guard.Until())
}

// CanStart reports whether a command identity which has never crossed the I/O
// boundary may make its first attempt. An unavailable Runtime can start one
// unprotected identity, but can never prove that identity safe to replay.
func (p ReplayPolicy) CanStart(guard replay.Guard) bool {
	if guard.Validate() != nil {
		return false
	}
	if guard.Protected() {
		return p.Replayable(guard)
	}
	return p.kind == policyUnavailable
}
