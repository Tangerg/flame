package plugin

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"

	"github.com/Tangerg/frame"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/content"
)

type ModelContextRequest struct {
	Use         flamemodel.Use `json:"use"`
	Provider    string         `json:"provider"`
	Model       string         `json:"model"`
	Request     chat.Request   `json:"request"`
	UserMessage *chat.Message  `json:"userMessage,omitzero"`
}

type ModelContextResult struct {
	Content []content.Block `json:"content"`
}

func ModelContextMethod() (*pluginmcp.Method[ModelContextRequest, ModelContextResult], error) {
	return pluginmcp.NewMethod[ModelContextRequest, ModelContextResult](frame.MethodConfig{Name: "flame.model-context/contribute.v3", Kind: frame.Query, Target: extension.ScopeCall, Description: "Contribute untrusted reference data for the captured model use and original user input before capacity admission."})
}

func ModelContextContract() (*plugincontract.Capability, error) { return modelContextContract() }

var modelContextContract = publishedCapability(func() (*plugincontract.Capability, error) {
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
