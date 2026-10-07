package mutation

import (
	"errors"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ReplayProfile supplies negotiated Runtime facts without transferring negotiation
// or connection ownership to the command workflow.
type ReplayProfile interface {
	Validate() error
	IdempotencyLimits() protocol.IdempotencyLimits
}

// PolicyFromProfile builds admission from the negotiated replay promise. The
// caller supplies the clock used for mutation admission.
func PolicyFromProfile(profile ReplayProfile, now func() time.Time) (ReplayPolicy, error) {
	if profile == nil {
		return ReplayPolicy{}, errors.New("runtime profile has not been negotiated")
	}
	if err := profile.Validate(); err != nil {
		return ReplayPolicy{}, err
	}
	limits := profile.IdempotencyLimits()
	capability, err := replay.NewCapability(limits.Namespace, time.Duration(limits.RetentionSeconds)*time.Second)
	if err != nil {
		return ReplayPolicy{}, err
	}
	return NewReplayPolicy(capability, now)
}
