package main

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"

	app "github.com/Tangerg/flame/plugins/memory/internal/application"

	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"
	"github.com/Tangerg/qimu/portable"
)

// errorCodes is ordered: an error joining several causes takes the first
// code listed, so classification never depends on map iteration.
var errorCodes = []struct {
	code  string
	cause error
}{
	{"workspace", app.ErrWorkspaceUnavailable},
	{"not_found", memory.ErrNotFound},
	{"not_visible", memory.ErrNotVisible},
	{"not_pending", memory.ErrNotPending},
	{"target_full", memory.ErrTargetFull},
	{"duplicate_content", memory.ErrDuplicateContent},
	{"invalid_input", app.ErrInvalidInput},
	{"revision_conflict", memory.ErrRevisionConflict},
	{"revision_exhausted", exactint.ErrExhausted},
	{"cursor", pagination.ErrInvalidCursor},
	{"limit", pagination.ErrInvalidLimit},
}

func codedCause(code string) error {
	for _, entry := range errorCodes {
		if entry.code == code {
			return entry.cause
		}
	}
	return nil
}

func classify(err error) (string, bool) {
	if failure, ok := errors.AsType[*portable.Refusal](err); ok {
		if codedCause(failure.Code) != nil {
			return failure.Code, true
		}
	}
	for _, entry := range errorCodes {
		if errors.Is(err, entry.cause) {
			return entry.code, true
		}
	}
	return "", false
}
