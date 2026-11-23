package authoring

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type RunOptions struct {
	Provider        string                    `json:"provider"`
	Model           string                    `json:"model"`
	ReasoningEffort string                    `json:"reasoningEffort"`
	Generation      protocol.GenerationParams `json:"generation"`
}

func (r RunOptions) Validate() error {
	var problems []error
	if err := model.ValidateSelection(r.Provider, r.Model, r.ReasoningEffort); err != nil {
		problems = append(problems, err)
	}
	if err := protocol.ValidateWireTree(r.Generation); err != nil {
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
