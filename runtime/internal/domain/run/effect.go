package run

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/scope/core/chat"
)

// UnresolvedEffect is evidence of an external operation, never an execution
// plan. Its owning Run may be canceled or timed out without resolving it.
type UnresolvedEffect struct {
	processID string
	effectID  string
	cause     string
	reason    string
	detail    string
	output    string
}

type UnresolvedEffectConfig struct {
	ProcessID string
	EffectID  string
	Cause     string
	Reason    string
	Detail    string
	Output    *chat.ToolOutput
}

func NewUnresolvedEffect(config UnresolvedEffectConfig) (UnresolvedEffect, error) {
	for _, identity := range []string{config.ProcessID, config.EffectID, config.Cause} {
		if strings.TrimSpace(identity) != identity || identity == "" || len(identity) > 512 {
			return UnresolvedEffect{}, errors.New("run: invalid unresolved effect identity or cause")
		}
	}
	if len(config.Reason) > 4096 || len(config.Detail) > 4096 {
		return UnresolvedEffect{}, errors.New("run: unresolved effect diagnostic exceeds limit")
	}
	var output string
	if config.Output != nil {
		encoded, err := json.Marshal(*config.Output)
		if err != nil {
			return UnresolvedEffect{}, fmt.Errorf("run: invalid unresolved effect output: %w", err)
		}
		output = string(encoded)
	}
	return UnresolvedEffect{processID: config.ProcessID, effectID: config.EffectID, cause: config.Cause, reason: config.Reason, detail: config.Detail, output: output}, nil
}
func (e UnresolvedEffect) ProcessID() string { return e.processID }
func (e UnresolvedEffect) EffectID() string  { return e.effectID }
func (e UnresolvedEffect) Cause() string     { return e.cause }
func (e UnresolvedEffect) Reason() string    { return e.reason }
func (e UnresolvedEffect) Detail() string    { return e.detail }

// The immutable Scope encoding isolates Run snapshots and equality from mutable
// content, media and metadata. Output remains evidence, never a settled result.
func (e UnresolvedEffect) Output() string { return e.output }

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
