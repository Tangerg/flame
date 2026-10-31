package main

import (
	"errors"

	routing "github.com/Tangerg/flame/plugins/modelrouter/internal/domain"
)

func classify(err error) (string, bool) {
	if errors.Is(err, errStoredPolicy) {
		return "", false
	}
	switch {
	case errors.Is(err, routing.ErrInvalidPolicy):
		return "invalid_policy", true
	case errors.Is(err, routing.ErrRevisionConflict):
		return "revision_conflict", true
	case errors.Is(err, routing.ErrRevisionLimit):
		return "revision_limit", true
	default:
		return "", false
	}
}
