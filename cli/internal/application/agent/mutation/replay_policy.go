package mutation

import (
	"errors"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
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
	capability commandreplay.Capability
	now        func() time.Time
}

func NewReplayPolicy(capability commandreplay.Capability, now func() time.Time) (ReplayPolicy, error) {
	if err := capability.Validate(); err != nil {
		return ReplayPolicy{}, err
	}
	if now == nil {
		return ReplayPolicy{}, errors.New("command replay policy clock is nil")
	}
	return ReplayPolicy{kind: policyAdvertised, capability: capability, now: now}, nil
}

func UnavailableReplayPolicy(now func() time.Time) (ReplayPolicy, error) {
	if now == nil {
		return ReplayPolicy{}, errors.New("command replay policy clock is nil")
	}
	return ReplayPolicy{kind: policyUnavailable, now: now}, nil
}

func (p ReplayPolicy) Validate() error {
	if p.now == nil {
		return errors.New("command replay policy clock is nil")
	}
	switch p.kind {
	case policyUnavailable:
		if p.capability != (commandreplay.Capability{}) {
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

func (p ReplayPolicy) NewGuard() (commandreplay.Guard, error) {
	return p.NewGuardAt(p.Now())
}

func (p ReplayPolicy) NewGuardAt(stagedAt time.Time) (commandreplay.Guard, error) {
	if err := p.Validate(); err != nil {
		return commandreplay.Guard{}, err
	}
	if p.kind == policyUnavailable {
		return commandreplay.UnprotectedGuard(), nil
	}
	until, err := p.capability.Deadline(stagedAt)
	if err != nil {
		return commandreplay.Guard{}, err
	}
	return commandreplay.NewProtectedGuard(p.capability.Namespace(), until)
}

func (p ReplayPolicy) SameStore(guard commandreplay.Guard) bool {
	if p.Validate() != nil || guard.Validate() != nil {
		return false
	}
	return p.kind == policyAdvertised && guard.Protected() &&
		guard.Namespace() == p.capability.Namespace()
}

func (p ReplayPolicy) Replayable(guard commandreplay.Guard) bool {
	return p.SameStore(guard) && p.Now().Before(guard.Until())
}

// CanStart reports whether a command identity which has never crossed the I/O
// boundary may make its first attempt. An unavailable Runtime can start one
// unprotected identity, but can never prove that identity safe to replay.
func (p ReplayPolicy) CanStart(guard commandreplay.Guard) bool {
	if p.Validate() != nil || guard.Validate() != nil {
		return false
	}
	if guard.Protected() {
		return p.Replayable(guard)
	}
	return p.kind == policyUnavailable
}
