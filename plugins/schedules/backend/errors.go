package main

import (
	"errors"

	app "github.com/Tangerg/flame/plugins/schedules/internal/application"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
)

var errorCodes = map[string]error{"workspace": app.ErrWorkspaceUnavailable, "command_conflict": extension.ErrInvocationConflict, "not_found": schedule.ErrNotFound, "revision": schedule.ErrRevisionConflict, "cursor": pagination.ErrInvalidCursor, "limit": pagination.ErrInvalidLimit, "invalid_cron": schedule.ErrInvalidCron, "model": modelchoice.ErrUnsupported, "instructions": schedule.ErrInstructionsRequired, "revision_required": schedule.ErrRevisionRequired}

func classify(err error) string {
	if modelchoice.IsInvalid(err) {
		return "model_input"
	}
	if failure, ok := errors.AsType[*extension.Refusal](err); ok {
		if errorCodes[failure.Code] != nil || failure.Code == "model_input" {
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

func decodeError(err error) error {
	var failure *extension.Refusal
	if errors.As(err, &failure) {
		if cause := errorCodes[failure.Code]; cause != nil {
			return errors.Join(cause, err)
		}
	}
	return err
}
