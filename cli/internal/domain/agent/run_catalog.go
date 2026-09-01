package agent

import (
	"errors"
	"fmt"

	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

// RunQuery selects one cursor page in runtime order, newest first. An empty
// status set means all lifecycle states; descendants remain opt-in because
// their presence changes both topology and pagination.
type RunQuery struct {
	SessionID          string
	Statuses           []runtimeprotocol.RunStatus
	IncludeDescendants bool
	Cursor             string
	PageSize           PageSize
}

func (r RunQuery) Validate() error {
	if r.SessionID != "" {
		if err := runtimeprotocol.ValidateSessionID(r.SessionID); err != nil {
			return fmt.Errorf("run query: %w", err)
		}
	}
	statuses := r.Statuses
	if len(statuses) == 0 {
		statuses = nil
	}
	if err := runtimeprotocol.ValidateWireTree(runtimeprotocol.ListRunsRequest{Statuses: statuses}); err != nil {
		return fmt.Errorf("run query statuses %q: %w", r.Statuses, err)
	}
	if _, err := r.PageSize.Rows(); err != nil {
		return fmt.Errorf("run query: %w", err)
	}
	return nil
}

type RunPage struct {
	Items      []Run
	NextCursor string
}

func (r RunPage) Validate() error {
	seen := make(map[string]struct{}, len(r.Items))
	for index, run := range r.Items {
		if err := run.Validate(); err != nil {
			return fmt.Errorf("run page item %d: %w", index+1, err)
		}
		if _, duplicate := seen[run.ID]; duplicate {
			return fmt.Errorf("run page repeats id %q", run.ID)
		}
		seen[run.ID] = struct{}{}
	}
	return nil
}

// RunCancellation is the atomic result of canceling one run-tree member.
// Canceled is always the addressed terminal run. Root is the authoritative
// root snapshot after that cancellation, which may still be active when only a
// child was canceled.
type RunCancellation struct {
	Canceled Run
	Root     Run
}

func (r RunCancellation) Validate() error {
	var problems []error
	if err := r.Canceled.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("canceled: %w", err))
	}
	if err := r.Root.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("root: %w", err))
	}
	if r.Canceled.Status != runtimeprotocol.RunStatusFinished || r.Canceled.Outcome.Status != OutcomeCanceled {
		problems = append(problems, errors.New("addressed run is not finished as canceled"))
	}
	if !r.Root.Lineage.IsRoot() {
		problems = append(problems, errors.New("root projection is a child run"))
	}
	if r.Canceled.SessionID != r.Root.SessionID {
		problems = append(problems, errors.New("canceled run and root belong to different sessions"))
	}
	if r.Canceled.Lineage.IsRoot() {
		if !r.Canceled.Equal(r.Root) {
			problems = append(problems, errors.New("root cancellation carries two different root projections"))
		}
	} else if r.Canceled.Lineage.RootRunID() != r.Root.ID {
		problems = append(problems, errors.New("canceled child does not belong to the returned root"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("run cancellation: %w", err)
	}
	return nil
}

func (r RunCancellation) ValidateTarget(runID string) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := runtimeprotocol.ValidateRunID(runID); err != nil {
		return fmt.Errorf("run cancellation: %w", err)
	}
	if r.Canceled.ID != runID {
		return fmt.Errorf("run cancellation: returned run %q, want %q", r.Canceled.ID, runID)
	}
	return nil
}
