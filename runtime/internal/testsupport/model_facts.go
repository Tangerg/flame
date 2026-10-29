package testsupport

import (
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/models/catalog"
)

func ModelFacts(selection modelchoice.Selection) *catalog.Model {
	entry, found := catalog.Default.Lookup(selection.Provider(), selection.Model())
	if !found {
		return nil
	}
	return &entry
}

func ModelTokenLimits(selection modelchoice.Selection) modelref.TokenLimits {
	entry := ModelFacts(selection)
	if entry == nil {
		return modelref.TokenLimits{}
	}
	known := func(value int64) *int64 {
		if value == 0 {
			return nil
		}
		return &value
	}
	limits, err := modelref.NewTokenLimits(modelref.TokenLimitValues{ContextWindow: known(entry.Limits.ContextWindow), MaxInputTokens: known(entry.Limits.MaxInputTokens), MaxOutputTokens: known(entry.Limits.MaxOutputTokens)})
	if err != nil {
		panic(err)
	}
	return limits
}
