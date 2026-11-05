package plugin

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/model"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/frame/portable"

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
func ModelSelectionMethod() (*pluginmcp.Method[ModelSelectionRequest, ModelSelection], error) {
	return pluginmcp.NewMethod[ModelSelectionRequest, ModelSelection](portable.MethodConfig{Name: "flame.model-selection/select.v2", Kind: portable.Query, Target: extension.ScopeCall, Description: "Select the exact model using the actual calling use and captured limits before provider construction, request-capacity admission and metering."})
}
func ModelSelectionContract() (*plugincontract.Capability, error) {
	method, err := ModelSelectionMethod()
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "flame.model-selection.v2", Composition: plugincontract.Selection, Scope: extension.ScopeCall}, method.Descriptor())
}
