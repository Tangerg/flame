package main

import (
	"errors"

	guardrail "github.com/Tangerg/flame/plugins/guardrails/internal/domain"
)

func classify(err error) (string, bool) {
	if errors.Is(err, errStoredPolicy) {
		return "", false
	}
	switch {
	case errors.Is(err, guardrail.ErrInvalidPolicy):
		return "invalid_policy", true
	case errors.Is(err, guardrail.ErrRevisionConflict):
		return "revision_conflict", true
	case errors.Is(err, guardrail.ErrRevisionLimit):
		return "revision_limit", true
	case errors.Is(err, guardrail.ErrProtectedContent):
		return "protected_content", true
	default:
		return "", false
	}
}
