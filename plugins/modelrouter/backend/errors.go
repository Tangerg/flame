package main

import (
	"errors"
	routing "github.com/Tangerg/flame/plugins/modelrouter/internal/domain"
	"github.com/Tangerg/flame/qimu/portable"
)

func classify(err error) string {
	if errors.Is(err, errStoredPolicy) {
		return ""
	}
	switch {
	case errors.Is(err, routing.ErrInvalidPolicy):
		return "invalid_policy"
	case errors.Is(err, routing.ErrRevisionConflict):
		return "revision_conflict"
	case errors.Is(err, routing.ErrRevisionLimit):
		return "revision_limit"
	case errors.Is(err, portable.ErrInvocationConflict):
		return "command_conflict"
	default:
		return ""
	}
}
