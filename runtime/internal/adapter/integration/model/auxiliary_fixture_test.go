package model

import (
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func auxiliaryFixture(model chat.Model) AuxiliaryModel {
	if dependency.Missing(model) {
		return AuxiliaryModel{}
	}
	resolved, err := NewResolvedChat(model, nil)
	if err != nil {
		panic(err)
	}
	capture, err := NewAuxiliaryModel(testsupport.DefaultModelSelection(), resolved)
	if err != nil {
		panic(err)
	}
	return capture
}
