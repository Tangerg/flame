package main

import (
	"errors"

	filewatch "github.com/Tangerg/flame/plugins/filewatch/internal/domain"
)

func classify(err error) (string, bool) {
	if errors.Is(err, errStored) {
		return "", false
	}
	switch {
	case errors.Is(err, filewatch.ErrInvalid):
		return "invalid_trigger", true
	case errors.Is(err, filewatch.ErrRevision):
		return "revision_conflict", true
	case errors.Is(err, filewatch.ErrNotFound):
		return "observation_not_found", true
	default:
		return "", false
	}
}
