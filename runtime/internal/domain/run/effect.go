package run

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

type UnresolvedEffect struct {
	processID string
	effectID  string
	cause     UnresolvedCause
	reason    string
	detail    string
	output    string
}

type UnresolvedCause string

const (
	UnresolvedAtTermination    UnresolvedCause = "termination"
	UnresolvedAtRuntimeFailure UnresolvedCause = "runtimeFailure"
)

func (c UnresolvedCause) Valid() bool {
	return c == UnresolvedAtTermination || c == UnresolvedAtRuntimeFailure
}

const maximumEffectIdentityBytes = 512

type UnresolvedEffectConfig struct {
	ProcessID string
	EffectID  string
	Cause     UnresolvedCause
	Reason    string
	Detail    string
	Output    *toolcontract.Output
}

func NewUnresolvedEffect(config UnresolvedEffectConfig) (UnresolvedEffect, error) {
	for _, identity := range []string{config.ProcessID, config.EffectID} {
		if strings.TrimSpace(identity) != identity || identity == "" || len(identity) > maximumEffectIdentityBytes {
			return UnresolvedEffect{}, errors.New("run: invalid unresolved effect identity")
		}
	}
	if !config.Cause.Valid() {
		return UnresolvedEffect{}, fmt.Errorf("run: unknown unresolved effect cause %q", config.Cause)
	}
	if len(config.Reason) > maximumDiagnosticBytes || len(config.Detail) > maximumDiagnosticBytes {
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

func (e UnresolvedEffect) ProcessID() string      { return e.processID }
func (e UnresolvedEffect) EffectID() string       { return e.effectID }
func (e UnresolvedEffect) Cause() UnresolvedCause { return e.cause }
func (e UnresolvedEffect) Reason() string         { return e.reason }
func (e UnresolvedEffect) Detail() string         { return e.detail }

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
