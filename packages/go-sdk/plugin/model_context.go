package plugin

import (
	"errors"
	"sync"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/content"
)

// ModelContextRequest describes the captured model and current request before
// context capacity admission. It does not authorize model execution or routing.
type ModelContextRequest struct {
	Use      modelchoice.Use `json:"use"`
	Provider string          `json:"provider"`
	Model    string          `json:"model"`
	Request  chat.Request    `json:"request"`
	// Host-authored user-role trailers cannot identify the original caller input.
	UserMessage *chat.Message `json:"userMessage,omitzero"`
}

// ModelContextResult supplies reference data, never message roles or instructions.
// Runtime frames it as untrusted material at the request tail, outside authored
// conversation. Contributors choose applicability from the captured Use.
type ModelContextResult struct {
	Content []content.Block `json:"content"`
}

func ModelContextMethod() (*pluginmcp.Method[ModelContextRequest, ModelContextResult], error) {
	return pluginmcp.NewMethod[ModelContextRequest, ModelContextResult](portable.MethodConfig{Name: "flame.model-context/contribute.v3", Kind: portable.Query, Target: extension.ScopeCall, Description: "Contribute untrusted reference data for the captured model use and original user input before capacity admission."})
}

// ModelContextContract orders independent context contributions before the final
// request is admitted. It grants no next proxy, model call or route replacement.
func ModelContextContract() (*plugincontract.Capability, error) { return modelContextContract() }

// The published contract is immutable. Checkpoint validation shares its compiled
// schemas instead of repeating admission on every durable tree commit.
var modelContextContract = sync.OnceValues(func() (*plugincontract.Capability, error) {
	method, err := ModelContextMethod()
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "flame.model-context.v3", Composition: plugincontract.Decoration, Scope: extension.ScopeCall}, method.Descriptor())
})

func (r ModelContextResult) Validate() error {
	for _, block := range r.Content {
		if err := block.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (r ModelContextRequest) Validate() error {
	if err := r.Use.Validate(); err != nil {
		return err
	}
	if err := (ModelSelection{Provider: r.Provider, Model: r.Model}).Validate(); err != nil {
		return err
	}
	if r.UserMessage != nil {
		if err := r.UserMessage.Validate(); err != nil {
			return err
		}
		if r.UserMessage.Role != chat.RoleUser {
			return errors.New("model context: original user input must have the user role")
		}
	}
	return r.Request.Validate()
}
