package main

import (
	"errors"

	app "github.com/Tangerg/flame/plugins/schedules/internal/application"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	"github.com/Tangerg/qimu/portable"
)

// errorCodes is ordered: an error joining several causes takes the first
// code listed, so classification never depends on map iteration.
var errorCodes = []struct {
	code  string
	cause error
}{
	{"workspace", app.ErrWorkspaceUnavailable},
	{"command_conflict", portable.ErrInvocationConflict},
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

func classify(err error) string {
	if modelchoice.IsInvalid(err) {
		return "model_input"
	}
	if failure, ok := errors.AsType[*portable.Refusal](err); ok {
		if codedCause(failure.Code) != nil || failure.Code == "model_input" {
			return failure.Code
		}
	}
	for _, entry := range errorCodes {
		if errors.Is(err, entry.cause) {
			return entry.code
		}
	}
	return ""
}

func decodeError(err error) error {
	var failure *portable.Refusal
	if errors.As(err, &failure) {
		if cause := codedCause(failure.Code); cause != nil {
			return errors.Join(cause, err)
		}
	}
	return err
}
