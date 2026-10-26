package main

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	routing "github.com/Tangerg/flame/plugins/modelrouter/internal/domain"
)

func classify(err error) string {
	if errors.Is(err, errStoredPolicy) {
		return ""
	}
	switch {
	case errors.Is(err, routing.ErrInvalidPolicy):
		return "invalid_policy"
	case errors.Is(err, routing.ErrRevisionConflict):
		return "revision_conflict"
	case errors.Is(err, routing.ErrRevisionLimit):
		return "revision_limit"
	case errors.Is(err, extension.ErrInvocationConflict):
		return "command_conflict"
	default:
		return ""
	}
}
