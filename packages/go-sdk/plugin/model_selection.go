package plugin

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/scope/core/chat"
)

// ModelSelection is the complete identity chosen before provider construction.
// Reasoning effort belongs to this exact provider/model, never to a later route.
type ModelSelection struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoningEffort,omitzero"`
}

func (s ModelSelection) Validate() error {
	selection, err := model.NewWithReasoningEffort(s.Provider, s.Model, s.ReasoningEffort)
	if err != nil {
		return err
	}
	return selection.ValidateExact()
}

// ModelSelectionRequest offers the caller's proposal before its exact binding.
// Request is absent when a use case captures its model before authoring input.
type ModelSelectionRequest struct {
	Context  model.SelectionContext `json:"context"`
	Proposed ModelSelection         `json:"proposed"`
	Request  *chat.Request          `json:"request,omitzero"`
}

func (r ModelSelectionRequest) Validate() error {
	if err := r.Context.Validate(); err != nil {
		return err
	}
	if err := r.Proposed.Validate(); err != nil {
		return err
	}
	if r.Request != nil {
		return r.Request.Validate()
	}
	return nil
}
func ModelSelectionMethod() (*Method[ModelSelectionRequest, ModelSelection], error) {
	return NewMethod[ModelSelectionRequest, ModelSelection](MethodConfig{Name: "flame.model-selection/select.v2", Kind: extension.Query, Target: "call", Description: "Select the exact model using the actual calling use and captured limits before provider construction, request-capacity admission and metering."})
}
func ModelSelectionContract() (*extension.Capability, error) {
	method, err := ModelSelectionMethod()
	if err != nil {
		return nil, err
	}
	return extension.NewCapability(extension.CapabilityConfig{Reference: "flame.model-selection.v2", Composition: extension.Selection, Scope: "call"}, method.Descriptor())
}
