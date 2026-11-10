package plugin

import (
	"errors"
	"sync"

	"github.com/Tangerg/frame/portable"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/scopewire"
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
func ModelInputMethod() (*pluginmcp.Method[ModelInputRequest, scopewire.Request], error) {
	return pluginmcp.NewMethod[ModelInputRequest, scopewire.Request](portable.MethodConfig{Name: "flame.model-input/transform.v1", Kind: portable.Query, Target: extension.ScopeCall, Description: "Transform the model-visible request before its final capacity admission while preserving the captured route and Tool contracts."})
}
func ModelInputContract() (*plugincontract.Capability, error) { return modelInputContract() }

// The published contract is immutable. Checkpoint validation shares its compiled
// schemas instead of repeating admission on every durable tree commit.
var modelInputContract = sync.OnceValues(func() (*plugincontract.Capability, error) {
	method, err := ModelInputMethod()
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "flame.model-input.v1", Composition: plugincontract.Decoration, Scope: extension.ScopeCall}, method.Descriptor())
})

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
	originalTools := make([]toolcontract.Definition, len(original.Tools))
	copy(originalTools, original.Tools)
	transformedTools := make([]toolcontract.Definition, len(transformed.Tools))
	copy(transformedTools, transformed.Tools)
	before, err := framevalue.EncodeJSON(struct {
		Tools  []toolcontract.Definition `json:"tools"`
		Choice *chat.ToolChoice          `json:"choice"`
	}{originalTools, original.ToolChoice})
	if err != nil {
		return err
	}
	after, err := framevalue.EncodeJSON(struct {
		Tools  []toolcontract.Definition `json:"tools"`
		Choice *chat.ToolChoice          `json:"choice"`
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
