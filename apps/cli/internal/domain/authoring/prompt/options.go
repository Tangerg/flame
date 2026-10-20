package prompt

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/protocol"
)

type RunOptions struct {
	Provider        string
	Model           string
	ReasoningEffort string
	Generation      protocol.GenerationParams
}

// Validate checks authored run options before they reach the Runtime. These
// values come from CLI flags, preferences and terminal forms, so they are the
// untrusted input this boundary owns.
func (r RunOptions) Validate() error {
	var problems []error
	if err := protocol.ValidateModelSelection(r.Provider, r.Model, r.ReasoningEffort); err != nil {
		problems = append(problems, err)
	}
	if err := r.Generation.ValidateWire(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("run options: %w", err)
	}
	return nil
}

func (r RunOptions) Clone() RunOptions {
	if r.Generation.Temperature != nil {
		r.Generation.Temperature = new(*r.Generation.Temperature)
	}
	if r.Generation.MaxTokens != nil {
		r.Generation.MaxTokens = new(*r.Generation.MaxTokens)
	}
	if r.Generation.TopP != nil {
		r.Generation.TopP = new(*r.Generation.TopP)
	}
	r.Generation.Stop = slices.Clone(r.Generation.Stop)
	return r
}

// Equal reports whether two run starts would carry the same complete execution
// configuration. Optional generation values retain nil-vs-zero semantics.
func (r RunOptions) Equal(other RunOptions) bool {
	return r.Provider == other.Provider && r.Model == other.Model && r.ReasoningEffort == other.ReasoningEffort &&
		equalOptional(r.Generation.Temperature, other.Generation.Temperature) &&
		equalOptional(r.Generation.MaxTokens, other.Generation.MaxTokens) &&
		equalOptional(r.Generation.TopP, other.Generation.TopP) &&
		slices.Equal(r.Generation.Stop, other.Generation.Stop)
}

func equalOptional[T comparable](left, right *T) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}
