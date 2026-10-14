package maintenance

import (
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func auxiliaryFixture(model chat.Model) modeladapter.AuxiliaryModel {
	if dependency.Missing(model) {
		return modeladapter.AuxiliaryModel{}
	}
	resolved, err := modeladapter.NewResolvedChat(model, nil)
	if err != nil {
		panic(err)
	}
	capture, err := modeladapter.NewAuxiliaryModel(testsupport.DefaultModelSelection(), resolved)
	if err != nil {
		panic(err)
	}
	return capture
}
