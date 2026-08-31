package commandreplay

import (
	"errors"
	"time"
)

type policyKind uint8

const (
	policyUnavailable policyKind = iota + 1
	policyAdvertised
)

// Policy joins the currently connected Runtime replay capability with the
// clock used to evaluate it. Unavailable is explicit and never reconstructed
// from empty capability fields.
type Policy struct {
	kind       policyKind
	capability Capability
	now        func() time.Time
}

func UnavailablePolicy() Policy {
	return Policy{kind: policyUnavailable, now: time.Now}
}

func NewPolicy(capability Capability) (Policy, error) {
	return NewPolicyWithClock(capability, time.Now)
}

func NewPolicyWithClock(capability Capability, now func() time.Time) (Policy, error) {
	if err := capability.Validate(); err != nil {
		return Policy{}, err
	}
	if now == nil {
		return Policy{}, errors.New("command replay policy clock is nil")
	}
	return Policy{kind: policyAdvertised, capability: capability, now: now}, nil
}

func UnavailablePolicyWithClock(now func() time.Time) (Policy, error) {
	if now == nil {
		return Policy{}, errors.New("command replay policy clock is nil")
	}
	return Policy{kind: policyUnavailable, now: now}, nil
}

func (p Policy) Validate() error {
	if p.now == nil {
		return errors.New("command replay policy clock is nil")
	}
	switch p.kind {
	case policyUnavailable:
		if p.capability != (Capability{}) {
			return errors.New("unavailable command replay policy carries a capability")
		}
		return nil
	case policyAdvertised:
		return p.capability.Validate()
	default:
		return errors.New("command replay policy is not configured")
	}
}

func (p Policy) Available() bool { return p.kind == policyAdvertised }

func (p Policy) Now() time.Time { return p.now().UTC() }

func (p Policy) NewGuard() (Guard, error) {
	return p.NewGuardAt(p.Now())
}

func (p Policy) NewGuardAt(stagedAt time.Time) (Guard, error) {
	if err := p.Validate(); err != nil {
		return Guard{}, err
	}
	if p.kind == policyUnavailable {
		return UnprotectedGuard(), nil
	}
	until, err := p.capability.Deadline(stagedAt)
	if err != nil {
		return Guard{}, err
	}
	return NewProtectedGuard(p.capability.Namespace(), until)
}

func (p Policy) SameStore(guard Guard) bool {
	if p.Validate() != nil || guard.Validate() != nil {
		return false
	}
	return p.kind == policyAdvertised && guard.Protected() &&
		guard.Namespace() == p.capability.Namespace()
}

func (p Policy) Replayable(guard Guard) bool {
	return p.SameStore(guard) && p.Now().Before(guard.Until())
}

// CanStart reports whether a command identity which has never crossed the I/O
// boundary may make its first attempt. An unavailable Runtime can start one
// unprotected identity, but can never prove that identity safe to replay.
func (p Policy) CanStart(guard Guard) bool {
	if p.Validate() != nil || guard.Validate() != nil {
		return false
	}
	if guard.Protected() {
		return p.Replayable(guard)
	}
	return p.kind == policyUnavailable
}
