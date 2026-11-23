package main

import (
	"errors"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/plugins/schedules/internal/schedule"
	"github.com/Tangerg/flame/plugins/schedules/internal/scheduling"

	framecall "github.com/Tangerg/frame/call"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
)

const modelInputCode = "model_input"

// Ordered so an error joining several causes takes the first listed code,
// independent of map iteration.
var errorCodes = []struct {
	code  string
	cause error
}{
	{"workspace", scheduling.ErrWorkspaceUnavailable},
	{"not_found", schedule.ErrNotFound},
	{"revision", schedule.ErrRevisionConflict},
	{"cursor", pagination.ErrInvalidCursor},
	{"limit", pagination.ErrInvalidLimit},
	{"invalid_cron", schedule.ErrInvalidCron},
	{"invalid_time_zone", schedule.ErrInvalidTimeZone},
	{"model", flamemodel.ErrUnsupported},
	{"instructions", schedule.ErrInstructionsRequired},
	{"revision_required", schedule.ErrRevisionRequired},
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
	if errors.Is(err, pluginmcp.ErrOutcomeUnconfirmed) {
		return "", false
	}
	if flamemodel.IsInvalid(err) {
		return modelInputCode, true
	}
	if failure, ok := errors.AsType[*framecall.Refusal](err); ok {
		if codedCause(failure.Code) != nil || failure.Code == modelInputCode {
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

func decodeError(err error) error {
	if errors.Is(err, pluginmcp.ErrOutcomeUnconfirmed) {
		return err
	}
	var failure *framecall.Refusal
	if errors.As(err, &failure) {
		if cause := codedCause(failure.Code); cause != nil {
			return errors.Join(cause, err)
		}
	}
	return err
}
