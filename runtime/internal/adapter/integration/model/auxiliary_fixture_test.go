package model

import (
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

func auxiliaryFixture(model chat.Model) SelectedChat {
	if lo.IsNil(model) {
		return SelectedChat{}
	}
	resolved, err := NewResolvedChat(model, nil, nil)
	if err != nil {
		panic(err)
	}
	capture, err := NewSelectedChat(testsupport.DefaultModelSelection(), resolved)
	if err != nil {
		panic(err)
	}
	return capture
}
