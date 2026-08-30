package modelcatalog

import (
	"fmt"

	"github.com/Tangerg/scope/models/catalog"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// LookupTokenLimits maps the static catalog envelope for one exact model into
// the Runtime's immutable Domain value. A catalog miss is not an error because
// configured compatible endpoints may legitimately expose private model IDs.
func LookupTokenLimits(selection modelref.Selection) (modelref.TokenLimits, bool, error) {
	if err := selection.Validate(); err != nil {
		return modelref.TokenLimits{}, false, fmt.Errorf("modelcatalog: token-limit selection: %w", err)
	}
	if !selection.Configured() {
		return modelref.TokenLimits{}, false, nil
	}
	entry, found := catalog.Default.Lookup(selection.Provider(), selection.Model())
	if !found {
		return modelref.TokenLimits{}, false, nil
	}
	limits, err := catalogTokenLimits(entry)
	if err != nil {
		return modelref.TokenLimits{}, false, fmt.Errorf(
			"modelcatalog: token limits for %q/%q: %w",
			selection.Provider(),
			selection.Model(),
			err,
		)
	}
	return limits, true, nil
}

func catalogTokenLimits(entry catalog.Model) (modelref.TokenLimits, error) {
	return modelref.NewTokenLimits(modelref.TokenLimitValues{
		ContextWindow:   publishedTokenLimit(entry.Limits.ContextWindow),
		MaxInputTokens:  publishedTokenLimit(entry.Limits.MaxInputTokens),
		MaxOutputTokens: publishedTokenLimit(entry.Limits.MaxOutputTokens),
	})
}

// publishedTokenLimit translates the upstream catalog's legacy numeric
// absence convention exactly once at the infrastructure boundary. Domain and
// application code never receive zero as an alternate spelling of unknown.
func publishedTokenLimit(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}
