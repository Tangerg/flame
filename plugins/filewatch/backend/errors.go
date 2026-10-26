package main

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	filewatch "github.com/Tangerg/flame/plugins/filewatch/internal/domain"
)

func classify(err error) string {
	if errors.Is(err, errStored) {
		return ""
	}
	switch {
	case errors.Is(err, filewatch.ErrInvalid):
		return "invalid_trigger"
	case errors.Is(err, filewatch.ErrRevision):
		return "revision_conflict"
	case errors.Is(err, filewatch.ErrNotFound):
		return "observation_not_found"
	case errors.Is(err, extension.ErrInvocationConflict):
		return "command_conflict"
	default:
		return ""
	}
}
