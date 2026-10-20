package maintenance

import (
	"github.com/Tangerg/flame/runtime/dependency"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func auxiliaryFixture(model chat.Model) modeladapter.SelectedChat {
	if dependency.Missing(model) {
		return modeladapter.SelectedChat{}
	}
	resolved, err := modeladapter.NewResolvedChat(model, nil)
	if err != nil {
		panic(err)
	}
	capture, err := modeladapter.NewSelectedChat(testsupport.DefaultModelSelection(), resolved)
	if err != nil {
		panic(err)
	}
	return capture
}
