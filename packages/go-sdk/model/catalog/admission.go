package catalog

import (
	"errors"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/scope/models/catalog"
)

type Admission struct {
	selection flamemodel.Selection
	metadata  *catalog.Model
}

func NewAdmission(selection flamemodel.Selection, metadata *catalog.Model) (Admission, error) {
	if err := selection.ValidateExact(); err != nil {
		return Admission{}, err
	}
	admitted := Admission{selection: selection}
	if metadata != nil {
		if err := metadata.Validate(); err != nil {
			return Admission{}, err
		}
		if metadata.ID != selection.Model() {
			return Admission{}, errors.New("model: catalog facts belong to another selection")
		}
		cloned := metadata.Clone()
		admitted.metadata = &cloned
	}
	if err := admitted.admitReasoning(); err != nil {
		return Admission{}, err
	}
	return admitted, nil
}

func (a Admission) Selection() flamemodel.Selection { return a.selection }
