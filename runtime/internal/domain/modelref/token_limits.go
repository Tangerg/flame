package modelref

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidTokenLimits reports a non-positive published fact or a prompt
	// maximum that exceeds a known context window.
	ErrInvalidTokenLimits = errors.New("model token limits: invalid")
	// ErrInvalidOutputReservation reports a non-positive explicit generation
	// reservation. Absence is represented by [OutputReservation] zero value,
	// never by the number zero.
	ErrInvalidOutputReservation = errors.New("model token limits: invalid output reservation")
	// ErrOutputTokenLimitExceeded reports an explicit generation request above
	// the selected model's published output maximum.
	ErrOutputTokenLimitExceeded = errors.New("model token limits: output maximum exceeded")
	// ErrOutputReservationExhaustsContext reports an explicit output reservation
	// that leaves no room for the non-empty model input.
	ErrOutputReservationExhaustsContext = errors.New("model token limits: output reservation exhausts context")
)

// TokenLimitValues is the explicit construction input for [TokenLimits]. A nil
// field means the provider did not publish that fact; every present value must
// be strictly positive.
type TokenLimitValues struct {
	ContextWindow   *int64
	MaxInputTokens  *int64
	MaxOutputTokens *int64
}

// TokenLimits is the immutable provider-published context envelope for one
// exact model. Its useful zero value means that no token-limit facts are known.
// Presence is stored independently from numeric values so zero can never leak
// into admission or compaction as an alternate spelling of "unknown".
//
// Input and output maxima are independent. Most chat providers count requested
// output inside the context window, while some streaming/multimodal models
// publish an output maximum larger than that window. That latter shape proves
// the window is an input envelope for that model and must not be treated as a
// shared input-plus-output budget.
type TokenLimits struct {
	contextWindow       int64
	contextWindowKnown  bool
	maxInputTokens      int64
	maxInputTokensKnown bool
	maxOutputTokens     int64
	maxOutputKnown      bool
}

// NewTokenLimits validates and freezes a model's published token-limit facts.
// An empty value is valid and produces the explicitly unknown policy.
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
	if err := limits.Validate(); err != nil {
		return TokenLimits{}, err
	}
	return limits, nil
}

// Validate checks the relationships that are knowable without inventing
// provider defaults. An unknown total window leaves independent maxima usable
// as facts, while a known total window bounds each maximum individually.
func (t TokenLimits) Validate() error {
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
			return fmt.Errorf("%w: %s must be positive", ErrInvalidTokenLimits, fact.name)
		}
	}
	if !t.contextWindowKnown {
		return nil
	}
	if t.maxInputTokensKnown && t.maxInputTokens > t.contextWindow {
		return fmt.Errorf(
			"%w: max input %d exceeds context window %d",
			ErrInvalidTokenLimits,
			t.maxInputTokens,
			t.contextWindow,
		)
	}
	return nil
}

// Unknown reports that the provider published no context-limit facts.
func (t TokenLimits) Unknown() bool {
	return !t.contextWindowKnown && !t.maxInputTokensKnown && !t.maxOutputKnown
}

// ContextWindow returns the provider-published context window and whether the
// provider published it. See [TokenLimits] for shared-vs-input-only semantics.
func (t TokenLimits) ContextWindow() (int64, bool) {
	return t.contextWindow, t.contextWindowKnown
}

// MaxInputTokens returns the published independent prompt maximum and whether
// the provider published it.
func (t TokenLimits) MaxInputTokens() (int64, bool) {
	return t.maxInputTokens, t.maxInputTokensKnown
}

// MaxOutputTokens returns the published independent generation maximum and
// whether the provider published it.
func (t TokenLimits) MaxOutputTokens() (int64, bool) {
	return t.maxOutputTokens, t.maxOutputKnown
}

// OutputReservation is the optional explicit generation ceiling for one model
// request. Its useful zero value means the caller did not request a ceiling.
type OutputReservation struct {
	tokens  int64
	present bool
}

// NewOutputReservation validates and freezes one explicit generation ceiling.
func NewOutputReservation(tokens int64) (OutputReservation, error) {
	if tokens <= 0 {
		return OutputReservation{}, fmt.Errorf(
			"%w: tokens must be positive",
			ErrInvalidOutputReservation,
		)
	}
	return OutputReservation{tokens: tokens, present: true}, nil
}

// Tokens returns the requested output ceiling and whether one was requested.
func (r OutputReservation) Tokens() (int64, bool) { return r.tokens, r.present }

// InputCeiling returns the hard prompt ceiling after reserving an explicitly
// requested output. The bool is false only when neither the provider input
// maximum nor a total context reservation establishes a hard input ceiling.
func (t TokenLimits) InputCeiling(reservation OutputReservation) (int64, bool, error) {
	if err := t.Validate(); err != nil {
		return 0, false, err
	}
	requestedOutput, requested := reservation.Tokens()
	if requested && t.maxOutputKnown && requestedOutput > t.maxOutputTokens {
		return 0, false, fmt.Errorf(
			"%w: requested %d, maximum %d",
			ErrOutputTokenLimitExceeded,
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
				ErrOutputReservationExhaustsContext,
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
