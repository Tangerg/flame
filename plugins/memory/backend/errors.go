package main

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"

	app "github.com/Tangerg/flame/plugins/memory/internal/application"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"
)

var errorCodes = map[string]error{"workspace": app.ErrWorkspaceUnavailable, "command_conflict": extension.ErrInvocationConflict,
	"not_found": memory.ErrNotFound, "not_visible": memory.ErrNotVisible, "not_pending": memory.ErrNotPending,
	"target_full": memory.ErrTargetFull, "duplicate_content": memory.ErrDuplicateContent, "invalid_input": app.ErrInvalidInput, "revision_conflict": memory.ErrRevisionConflict, "revision_exhausted": exactint.ErrExhausted, "cursor": pagination.ErrInvalidCursor, "limit": pagination.ErrInvalidLimit,
}

func classify(err error) string {
	if failure, ok := errors.AsType[*extension.Refusal](err); ok {
		if errorCodes[failure.Code] != nil {
			return failure.Code
		}
	}
	for code, cause := range errorCodes {
		if errors.Is(err, cause) {
			return code
		}
	}
	return ""
}
