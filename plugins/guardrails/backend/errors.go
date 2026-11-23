package main

import (
	"errors"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/guardrails/internal/guardrail"
)

func classify(err error) (string, bool) {
	if errors.Is(err, pluginstorage.ErrStoredSnapshot) {
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
