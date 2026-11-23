package modelref

import (
	"errors"
	"fmt"
)

var (
	errInvalidTokenLimits               = errors.New("model token limits: invalid")
	errInvalidOutputReservation         = errors.New("model token limits: invalid output reservation")
	errOutputTokenLimitExceeded         = errors.New("model token limits: output maximum exceeded")
	errOutputReservationExhaustsContext = errors.New("model token limits: output reservation exhausts context")
)

type TokenLimitValues struct {
	ContextWindow   *int64
	MaxInputTokens  *int64
	MaxOutputTokens *int64
}

// Some streaming/multimodal models publish an output maximum larger than the
// context window, so the window is an input envelope and must not be treated as a
// shared input-plus-output budget.
type TokenLimits struct {
	contextWindow       int64
	contextWindowKnown  bool
	maxInputTokens      int64
	maxInputTokensKnown bool
	maxOutputTokens     int64
	maxOutputKnown      bool
}

func NewTokenLimits(values TokenLimitValues) (TokenLimits, error) {
	limits := TokenLimits{}
	if values.ContextWindow != nil {
		limits.contextWindow = *values.ContextWindow
		limits.contextWindowKnown = true
	}
	if values.MaxInputTokens != nil {
		limits.maxInputTokens = *values.MaxInputTokens
		limits.maxInputTokensKnown = true
	}
	if values.MaxOutputTokens != nil {
		limits.maxOutputTokens = *values.MaxOutputTokens
		limits.maxOutputKnown = true
	}
	if err := limits.validate(); err != nil {
		return TokenLimits{}, err
	}
	return limits, nil
}

func (t TokenLimits) validate() error {
	for _, fact := range []struct {
		name    string
		value   int64
		present bool
	}{
		{name: "context window", value: t.contextWindow, present: t.contextWindowKnown},
		{name: "max input", value: t.maxInputTokens, present: t.maxInputTokensKnown},
		{name: "max output", value: t.maxOutputTokens, present: t.maxOutputKnown},
	} {
		if fact.present && fact.value <= 0 {
			return fmt.Errorf("%w: %s must be positive", errInvalidTokenLimits, fact.name)
		}
	}
	if !t.contextWindowKnown {
		return nil
	}
	if t.maxInputTokensKnown && t.maxInputTokens > t.contextWindow {
		return fmt.Errorf(
			"%w: max input %d exceeds context window %d",
			errInvalidTokenLimits,
			t.maxInputTokens,
			t.contextWindow,
		)
	}
	return nil
}

func (t TokenLimits) Unknown() bool {
	return !t.contextWindowKnown && !t.maxInputTokensKnown && !t.maxOutputKnown
}

func (t TokenLimits) ContextWindow() (int64, bool) {
	return t.contextWindow, t.contextWindowKnown
}

func (t TokenLimits) MaxInputTokens() (int64, bool) {
	return t.maxInputTokens, t.maxInputTokensKnown
}

func (t TokenLimits) MaxOutputTokens() (int64, bool) {
	return t.maxOutputTokens, t.maxOutputKnown
}

type OutputReservation struct {
	tokens  int64
	present bool
}

func NewOutputReservation(tokens int64) (OutputReservation, error) {
	if tokens <= 0 {
		return OutputReservation{}, fmt.Errorf(
			"%w: tokens must be positive",
			errInvalidOutputReservation,
		)
	}
	return OutputReservation{tokens: tokens, present: true}, nil
}

func (r OutputReservation) Tokens() (int64, bool) { return r.tokens, r.present }

func (t TokenLimits) InputCeiling(reservation OutputReservation) (int64, bool, error) {
	requestedOutput, requested := reservation.Tokens()
	if requested && t.maxOutputKnown && requestedOutput > t.maxOutputTokens {
		return 0, false, fmt.Errorf(
			"%w: requested %d, maximum %d",
			errOutputTokenLimitExceeded,
			requestedOutput,
			t.maxOutputTokens,
		)
	}

	ceiling, known := t.maxInputTokens, t.maxInputTokensKnown
	sharesOutput := !t.maxOutputKnown || t.maxOutputTokens <= t.contextWindow
	if requested && t.contextWindowKnown && sharesOutput {
		if requestedOutput >= t.contextWindow {
			return 0, false, fmt.Errorf(
				"%w: requested %d, context window %d",
				errOutputReservationExhaustsContext,
				requestedOutput,
				t.contextWindow,
			)
		}
		remaining := t.contextWindow - requestedOutput
		if !known || remaining < ceiling {
			ceiling, known = remaining, true
		}
	}
	return ceiling, known, nil
}
