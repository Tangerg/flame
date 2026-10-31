package main

import (
	"errors"

	processsandbox "github.com/Tangerg/flame/packages/go-sdk/sandbox"
	sandbox "github.com/Tangerg/flame/plugins/sandbox/internal/domain"
)

func classify(err error) (string, bool) {
	if errors.Is(err, processsandbox.ErrUnavailable) {
		return "sandbox_unavailable", true
	}
	switch {
	case errors.Is(err, sandbox.ErrInvalidProgram):
		return "invalid_program", true
	default:
		return "", false
	}
}
