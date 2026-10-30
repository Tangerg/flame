package pluginservices

import (
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

func auxiliaryFixture(model chat.Model) modeladapter.SelectedChat {
	if lo.IsNil(model) {
		return modeladapter.SelectedChat{}
	}
	resolved, err := modeladapter.NewResolvedChat(model, nil, nil)
	if err != nil {
		panic(err)
	}
	capture, err := modeladapter.NewSelectedChat(testsupport.DefaultModelSelection(), resolved)
	if err != nil {
		panic(err)
	}
	return capture
}
