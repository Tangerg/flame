package schedules

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/dependency"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
)

type RunReference struct{ SessionID, RunID string }

// RunAdmission returns nil only after the original captured Session/Run opening
// is confirmed by Runtime, including when its material has been retired.
type RunAdmission interface {
	Admit(context.Context, schedule.RunRequest) error
}

type DispatchStore interface {
	CapturedRun(context.Context, string) (schedule.RunRequest, error)
	AcceptRun(context.Context, string) error
}

type Dispatcher struct {
	store DispatchStore
	runs  RunAdmission
}

func NewDispatcher(store DispatchStore, runs RunAdmission) (*Dispatcher, error) {
	if dependency.Missing(store) || dependency.Missing(runs) {
		return nil, errors.New("schedules: dispatch store and Run admission are required")
	}
	return &Dispatcher{store: store, runs: runs}, nil
}

func (d *Dispatcher) Dispatch(ctx context.Context, id string) (RunReference, error) {
	request, err := d.store.CapturedRun(ctx, id)
	if err != nil {
		return RunReference{}, err
	}
	if err := request.Validate(); err != nil {
		return RunReference{}, err
	}
	if request.RunID() != id {
		return RunReference{}, errors.New("schedules: captured dispatch identity disagrees")
	}
	if err := d.runs.Admit(ctx, request); err != nil {
		return RunReference{}, err
	}
	if err := d.store.AcceptRun(ctx, id); err != nil {
		return RunReference{}, err
	}
	return RunReference{SessionID: request.SessionID(), RunID: request.RunID()}, nil
}
