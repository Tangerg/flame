package main

import (
	"errors"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/plugins/memory/internal/agentmemory"
	"github.com/Tangerg/flame/plugins/memory/internal/coordination"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	framecall "github.com/Tangerg/frame/call"

	"github.com/Tangerg/flame/packages/go-sdk/pagination"
)

// Ordered so an error joining several causes takes the first listed code,
// independent of map iteration.
var errorCodes = []struct {
	code  string
	cause error
}{
	{"workspace", coordination.ErrWorkspaceUnavailable},
	{"not_found", agentmemory.ErrNotFound},
	{"not_visible", agentmemory.ErrNotVisible},
	{"not_pending", agentmemory.ErrNotPending},
	{"target_full", agentmemory.ErrTargetFull},
	{"duplicate_content", agentmemory.ErrDuplicateContent},
	{pluginmcp.InvalidInputCode, coordination.ErrInvalidInput},
	{"invalid_facts", agentmemory.ErrInvalidFacts},
	{"revision_conflict", agentmemory.ErrRevisionConflict},
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
	if failure, ok := errors.AsType[*framecall.Refusal](err); ok {
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
