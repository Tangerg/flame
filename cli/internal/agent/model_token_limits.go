package agent

import (
	"errors"
	"fmt"
)

var ErrInvalidModelTokenLimits = errors.New("model token limits: invalid")

// ModelTokenLimitValues is the explicit construction input for
// [ModelTokenLimits]. Nil means the provider did not publish that fact; every
// present value must be strictly positive.
type ModelTokenLimitValues struct {
	ContextWindow   *int64
	MaxInputTokens  *int64
	MaxOutputTokens *int64
}

// ModelTokenLimits is the CLI application's immutable view of one model's
// provider-published context envelope. Its useful zero value means no facts are
// known; numeric zero is never an alternate spelling of absence.
type ModelTokenLimits struct {
	contextWindow       int64
	contextWindowKnown  bool
	maxInputTokens      int64
	maxInputTokensKnown bool
	maxOutputTokens     int64
	maxOutputKnown      bool
}

func NewModelTokenLimits(values ModelTokenLimitValues) (ModelTokenLimits, error) {
	limits := ModelTokenLimits{}
	if values.ContextWindow != nil {
		limits.contextWindow, limits.contextWindowKnown = *values.ContextWindow, true
	}
	if values.MaxInputTokens != nil {
		limits.maxInputTokens, limits.maxInputTokensKnown = *values.MaxInputTokens, true
	}
	if values.MaxOutputTokens != nil {
		limits.maxOutputTokens, limits.maxOutputKnown = *values.MaxOutputTokens, true
	}
	if err := limits.Validate(); err != nil {
		return ModelTokenLimits{}, err
	}
	return limits, nil
}

func (m ModelTokenLimits) Validate() error {
	for _, fact := range []struct {
		name    string
		value   int64
		present bool
	}{
		{name: "context window", value: m.contextWindow, present: m.contextWindowKnown},
		{name: "max input", value: m.maxInputTokens, present: m.maxInputTokensKnown},
		{name: "max output", value: m.maxOutputTokens, present: m.maxOutputKnown},
	} {
		if fact.present && fact.value <= 0 {
			return fmt.Errorf("%w: %s must be positive", ErrInvalidModelTokenLimits, fact.name)
		}
	}
	if !m.contextWindowKnown {
		return nil
	}
	if m.maxInputTokensKnown && m.maxInputTokens > m.contextWindow {
		return fmt.Errorf("%w: max input exceeds context window", ErrInvalidModelTokenLimits)
	}
	return nil
}

func (m ModelTokenLimits) Unknown() bool {
	return !m.contextWindowKnown && !m.maxInputTokensKnown && !m.maxOutputKnown
}

func (m ModelTokenLimits) ContextWindow() (int64, bool) {
	return m.contextWindow, m.contextWindowKnown
}

func (m ModelTokenLimits) MaxInputTokens() (int64, bool) {
	return m.maxInputTokens, m.maxInputTokensKnown
}

func (m ModelTokenLimits) MaxOutputTokens() (int64, bool) {
	return m.maxOutputTokens, m.maxOutputKnown
}
