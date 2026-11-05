package plugin

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/frame/portable"
)

const FeedbackReference = "flame.feedback.v1"

type FeedbackSignal struct {
	Rating protocol.FeedbackRating `json:"rating,omitempty"`
	Text   string                  `json:"text,omitempty"`
}

func PrepareFeedbackMethod() (*pluginmcp.Method[extension.FeedbackDraft, FeedbackSignal], error) {
	return pluginmcp.NewMethod[extension.FeedbackDraft, FeedbackSignal](portable.MethodConfig{
		Name: "flame.feedback/prepare.v1", Kind: portable.Command, Target: extension.ScopeInstallation,
		Description: "Prepare the immutable quality observation for this command and retain its original decision.",
	})
}

func FeedbackContract() (*plugincontract.Capability, error) {
	method, err := PrepareFeedbackMethod()
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: FeedbackReference, Composition: plugincontract.Selection, Scope: extension.ScopeInstallation}, method.Descriptor())
}
