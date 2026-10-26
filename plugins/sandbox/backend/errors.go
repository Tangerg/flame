package main

import (
	"errors"

	processsandbox "github.com/Tangerg/flame/packages/go-sdk/sandbox"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	sandbox "github.com/Tangerg/flame/plugins/sandbox/internal/domain"
)

func classify(err error) string {
	if errors.Is(err, processsandbox.ErrUnavailable) {
		return "sandbox_unavailable"
	}
	switch {
	case errors.Is(err, sandbox.ErrInvalidProgram):
		return "invalid_program"
	case errors.Is(err, extension.ErrInvocationConflict):
		return "command_conflict"
	default:
		return ""
	}
}
