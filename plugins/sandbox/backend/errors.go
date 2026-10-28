package main

import (
	"errors"
	"github.com/Tangerg/flame/qimu/portable"

	processsandbox "github.com/Tangerg/flame/packages/go-sdk/sandbox"
	sandbox "github.com/Tangerg/flame/plugins/sandbox/internal/domain"
)

func classify(err error) string {
	if errors.Is(err, processsandbox.ErrUnavailable) {
		return "sandbox_unavailable"
	}
	switch {
	case errors.Is(err, sandbox.ErrInvalidProgram):
		return "invalid_program"
	case errors.Is(err, portable.ErrInvocationConflict):
		return "command_conflict"
	default:
		return ""
	}
}
