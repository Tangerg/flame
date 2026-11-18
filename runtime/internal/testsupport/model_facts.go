package testsupport

import (
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/scope/models/catalog"
)

func ModelFacts(selection modelchoice.Selection) *catalog.Model {
	entry, found := catalog.LookupModel(selection.Provider(), selection.Model())
	if !found {
		return nil
	}
	return &entry
}
