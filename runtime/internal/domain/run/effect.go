package run

import (
	"errors"
	"strings"
)

// UnresolvedEffect is evidence of an external operation, never an execution
// plan. Its owning Run may be canceled or timed out without resolving it.
type UnresolvedEffect struct {
	processID string
	effectID  string
	cause     string
	reason    string
	detail    string
}

func NewUnresolvedEffect(processID, effectID, cause, reason, detail string) (UnresolvedEffect, error) {
	for _, identity := range []string{processID, effectID, cause} {
		if strings.TrimSpace(identity) != identity || identity == "" || len(identity) > 512 {
			return UnresolvedEffect{}, errors.New("run: invalid unresolved effect identity or cause")
		}
	}
	if len(reason) > 4096 || len(detail) > 4096 {
		return UnresolvedEffect{}, errors.New("run: unresolved effect diagnostic exceeds limit")
	}
	return UnresolvedEffect{processID: processID, effectID: effectID, cause: cause, reason: reason, detail: detail}, nil
}
func (e UnresolvedEffect) ProcessID() string { return e.processID }
func (e UnresolvedEffect) EffectID() string  { return e.effectID }
func (e UnresolvedEffect) Cause() string     { return e.cause }
func (e UnresolvedEffect) Reason() string    { return e.reason }
func (e UnresolvedEffect) Detail() string    { return e.detail }

func validateUnresolvedEffects(effects []UnresolvedEffect) error {
	seen := make(map[string]bool, len(effects))
	for _, effect := range effects {
		if effect.effectID == "" || seen[effect.effectID] {
			return errors.New("run: invalid or duplicate unresolved effect")
		}
		seen[effect.effectID] = true
	}
	return nil
}
