package commandreplay

import (
	"errors"
	"time"
)

// Policy joins the connected Runtime replay capability with the clock used to
// evaluate the store-bound deadline before each mutation attempt.
type Policy struct {
	capability Capability
	now        func() time.Time
}

func NewPolicyWithClock(capability Capability, now func() time.Time) (Policy, error) {
	if err := capability.Validate(); err != nil {
		return Policy{}, err
	}
	if now == nil {
		return Policy{}, errors.New("command replay policy clock is nil")
	}
	return Policy{capability: capability, now: now}, nil
}

func (p Policy) Validate() error {
	if p.now == nil {
		return errors.New("command replay policy clock is nil")
	}
	return p.capability.Validate()
}

func (p Policy) Now() time.Time { return p.now().UTC() }

func (p Policy) NewGuard() (Guard, error) {
	if err := p.Validate(); err != nil {
		return Guard{}, err
	}
	return p.NewGuardAt(p.Now())
}

func (p Policy) NewGuardAt(stagedAt time.Time) (Guard, error) {
	if err := p.Validate(); err != nil {
		return Guard{}, err
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
	return guard.Protected() &&
		guard.Namespace() == p.capability.Namespace()
}

func (p Policy) Replayable(guard Guard) bool {
	return p.SameStore(guard) && p.Now().Before(guard.Until())
}
