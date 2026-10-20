package model

import (
	"github.com/Tangerg/flame/runtime/dependency"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func auxiliaryFixture(model chat.Model) SelectedChat {
	if dependency.Missing(model) {
		return SelectedChat{}
	}
	resolved, err := NewResolvedChat(model, nil)
	if err != nil {
		panic(err)
	}
	capture, err := NewSelectedChat(testsupport.DefaultModelSelection(), resolved)
	if err != nil {
		panic(err)
	}
	return capture
}
