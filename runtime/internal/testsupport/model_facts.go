package testsupport

import (
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/scope/models/catalog"
)

func ModelFacts(selection flamemodel.Selection) *catalog.Model {
	entry, found := catalog.LookupModel(selection.Provider(), selection.Model())
	if !found {
		return nil
	}
	return &entry
}
