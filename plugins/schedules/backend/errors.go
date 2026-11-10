package main

import (
	"errors"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"

	framemcp "github.com/Tangerg/frame/process/mcp"

	app "github.com/Tangerg/flame/plugins/schedules/internal/application"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
)

// modelInputCode reports a model selection the model contract rejects; it has
// no single sentinel, so the table cannot name it.
const modelInputCode = "model_input"

// errorCodes is ordered: an error joining several causes takes the first
// code listed, so classification never depends on map iteration.
var errorCodes = []struct {
	code  string
	cause error
}{
	{"workspace", app.ErrWorkspaceUnavailable},
	{"not_found", schedule.ErrNotFound},
	{"revision", schedule.ErrRevisionConflict},
	{"cursor", pagination.ErrInvalidCursor},
	{"limit", pagination.ErrInvalidLimit},
	{"invalid_cron", schedule.ErrInvalidCron},
	{"invalid_time_zone", schedule.ErrInvalidTimeZone},
	{"model", modelchoice.ErrUnsupported},
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
	if modelchoice.IsInvalid(err) {
		return modelInputCode, true
	}
	if failure, ok := errors.AsType[*framemcp.Refusal](err); ok {
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
	var failure *framemcp.Refusal
	if errors.As(err, &failure) {
		if cause := codedCause(failure.Code); cause != nil {
			return errors.Join(cause, err)
		}
	}
	return err
}
