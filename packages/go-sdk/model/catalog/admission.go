package catalog

import (
	"errors"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/scope/models/catalog"
)

// Admission freezes the facts supplied by the selected model's owner. Absence
// keeps capability unknown; it never substitutes a process-wide vendor catalog.
type Admission struct {
	selection modelchoice.Selection
	metadata  *catalog.Model
}

func NewAdmission(selection modelchoice.Selection, metadata *catalog.Model) (Admission, error) {
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
		copy := metadata.Clone()
		admitted.metadata = &copy
	}
	if err := admitted.admitReasoning(); err != nil {
		return Admission{}, err
	}
	return admitted, nil
}

func (a Admission) Selection() modelchoice.Selection { return a.selection }
