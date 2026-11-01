package plugin

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	qimumcp "github.com/Tangerg/qimu/mcp"
	"github.com/Tangerg/qimu/portable"
)

const FeedbackReference = "flame.feedback.v1"

type FeedbackSignal struct {
	Rating protocol.FeedbackRating `json:"rating,omitempty"`
	Text   string                  `json:"text,omitempty"`
}

func PrepareFeedbackMethod() (*qimumcp.Method[extension.FeedbackDraft, FeedbackSignal], error) {
	return qimumcp.NewMethod[extension.FeedbackDraft, FeedbackSignal](qimumcp.MethodConfig{
		Name: "flame.feedback/prepare.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation,
		Description: "Prepare the immutable quality observation for this command and retain its original decision.",
	})
}

func FeedbackContract() (*portable.Capability, error) {
	method, err := PrepareFeedbackMethod()
	if err != nil {
		return nil, err
	}
	return portable.NewCapability(portable.CapabilityConfig{Reference: FeedbackReference, Composition: portable.Selection, Scope: extension.ScopeInstallation}, method.Descriptor())
}
