package main

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	guardrail "github.com/Tangerg/flame/plugins/guardrails/internal/domain"
)

func classify(err error) string {
	if errors.Is(err, errStoredPolicy) {
		return ""
	}
	switch {
	case errors.Is(err, guardrail.ErrInvalidPolicy):
		return "invalid_policy"
	case errors.Is(err, guardrail.ErrRevisionConflict):
		return "revision_conflict"
	case errors.Is(err, guardrail.ErrRevisionLimit):
		return "revision_limit"
	case errors.Is(err, guardrail.ErrProtectedContent):
		return "protected_content"
	case errors.Is(err, extension.ErrInvocationConflict):
		return "command_conflict"
	default:
		return ""
	}
}
