package main

import (
	"errors"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	routing "github.com/Tangerg/flame/plugins/modelrouter/internal/domain"
)

func classify(err error) (string, bool) {
	if errors.Is(err, pluginstorage.ErrStoredSnapshot) {
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
