package plugin

import (
	"errors"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/scope/core/chat"
)

// ModelInputRequest carries one captured route and the current model-visible
// input. Transformation grants no routing, Tool contract or execution authority.
type ModelInputRequest struct {
	Selection ModelSelection `json:"selection"`
	Request   chat.Request   `json:"request"`
}

func (r ModelInputRequest) Validate() error {
	if err := r.Selection.Validate(); err != nil {
		return err
	}
	return r.Request.Validate()
}
func ModelInputMethod() (*Method[ModelInputRequest, chat.Request], error) {
	return NewMethod[ModelInputRequest, chat.Request](MethodConfig{Name: "flame.model-input/transform.v1", Kind: capability.Query, Target: "call", Description: "Transform the model-visible request before its final capacity admission while preserving the captured route and Tool contracts."})
}
func ModelInputContract() (*capability.Contract, error) {
	method, err := ModelInputMethod()
	if err != nil {
		return nil, err
	}
	return capability.New(capability.Config{Reference: "flame.model-input.v1", Composition: capability.Decoration, Scope: "call"}, method.Descriptor())
}

// ValidateModelInput preserves facts owned by selection, Tool preparation and
// the caller's resource envelope. The owner admits transformed media/capacity.
func ValidateModelInput(original, transformed *chat.Request) error {
	if original == nil || transformed == nil {
		return errors.New("model input: original and transformed requests are required")
	}
	if err := original.Validate(); err != nil {
		return err
	}
	if err := transformed.Validate(); err != nil {
		return err
	}
	if transformed.Options.Model != original.Options.Model || transformed.Options.ReasoningEffort != original.Options.ReasoningEffort {
		return errors.New("model input: transformation cannot replace the captured model options")
	}
	originalTools := make([]chat.ToolDefinition, len(original.Tools))
	copy(originalTools, original.Tools)
	transformedTools := make([]chat.ToolDefinition, len(transformed.Tools))
	copy(transformedTools, transformed.Tools)
	before, err := jsoninput.Encode(struct {
		Tools  []chat.ToolDefinition `json:"tools"`
		Choice *chat.ToolChoice      `json:"choice"`
	}{originalTools, original.ToolChoice})
	if err != nil {
		return err
	}
	after, err := jsoninput.Encode(struct {
		Tools  []chat.ToolDefinition `json:"tools"`
		Choice *chat.ToolChoice      `json:"choice"`
	}{transformedTools, transformed.ToolChoice})
	if err != nil {
		return err
	}
	if before != after {
		return errors.New("model input: transformation cannot replace Tool contracts or choice")
	}
	reserved := original.Options.MaxOutputTokens
	output := transformed.Options.MaxOutputTokens
	if (reserved == nil) != (output == nil) || (reserved != nil && *output > *reserved) {
		return errors.New("model input: transformation cannot increase or replace the original output envelope")
	}
	return nil
}
