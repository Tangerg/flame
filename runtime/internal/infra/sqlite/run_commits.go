package sqlite

import (
	"context"
	"fmt"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// RecordRunCommit stamps one exact active Segment's latest immutable
// Application write-set identity into the Run row. Callers invoke it only at
// the end of the command transaction, after every projection has succeeded.
func (r *RunStore) RecordRunCommit(
	ctx context.Context,
	sessionID string,
	runID string,
	segmentID string,
	commitID runtimeidentity.CommitID,
) error {
	if err := validateRunCommitIdentity(sessionID, runID, segmentID, commitID); err != nil {
		return err
	}
	result, err := conn(ctx, r.db).ExecContext(ctx,
		`UPDATE runs SET commit_segment_id = ?, commit_id = ?
		  WHERE session_id = ? AND run_id = ? AND state = ? AND active_segment_id = ?`,
		segmentID,
		commitID.String(),
		sessionID,
		runID,
		runStateRunning.databaseValue(),
		segmentID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: record Run commit %q: %w", commitID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect Run commit %q marker: %w", commitID, err)
	}
	if changed != 1 {
		return fmt.Errorf("sqlite: Run commit %q lost its active-segment fence", commitID)
	}
	return nil
}

// RecordWaitingRunCommit stamps a command that transforms an already-waiting
// tree without opening a new Segment. The empty Segment is deliberate: the
// unique command identity and Waiting root own this boundary.
func (r *RunStore) RecordWaitingRunCommit(
	ctx context.Context,
	sessionID string,
	runID string,
	commitID runtimeidentity.CommitID,
) error {
	if err := validateWaitingRunCommitIdentity(sessionID, runID, commitID); err != nil {
		return err
	}
	result, err := conn(ctx, r.db).ExecContext(ctx,
		`UPDATE runs SET commit_segment_id = '', commit_id = ?
		  WHERE session_id = ? AND run_id = ? AND state = ? AND active_segment_id = ''`,
		commitID.String(),
		sessionID,
		runID,
		runStateWaiting.databaseValue(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: record waiting Run commit %q: %w", commitID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect waiting Run commit %q marker: %w", commitID, err)
	}
	if changed != 1 {
		return fmt.Errorf("sqlite: waiting Run commit %q lost its state fence", commitID)
	}
	return nil
}

// runCommitMarker is the optional durable fence attached to one Application
// write-set. A non-nil marker always carries both coordinates because callers
// can only construct it through newRunCommitMarker; nil means the lifecycle
// transition carries no individual receipt, as with a child in a root-owned
// tree barrier.
type runCommitMarker struct {
	segmentID string
	commitID  runtimeidentity.CommitID
}

func newRunCommitMarker(
	sessionID string,
	runID string,
	segmentID string,
	commitID runtimeidentity.CommitID,
) (*runCommitMarker, error) {
	if err := validateRunCommitIdentity(sessionID, runID, segmentID, commitID); err != nil {
		return nil, err
	}
	return &runCommitMarker{segmentID: segmentID, commitID: commitID}, nil
}

func (r *runCommitMarker) requireActiveSegment(activeSegmentID string) error {
	if r == nil {
		return nil
	}
	if activeSegmentID != r.segmentID {
		return fmt.Errorf("active Segment is %q, want %q", activeSegmentID, r.segmentID)
	}
	return nil
}

func (r *runCommitMarker) databaseValues() (string, string) {
	if r == nil {
		return "", ""
	}
	return r.segmentID, r.commitID.String()
}

// RunCommitCommitted proves that this exact immutable Application Run write-set crossed
// the durable boundary. It does not infer success from the coarse Run state:
// another Segment, restored/resumed Run, or later write attempt has a different
// or absent marker. Running markers require the same active Segment; waiting
// barriers and terminal boundaries retain the Segment that produced them.
// A command that starts and ends while already Waiting uses an empty Segment.
func (r *RunStore) RunCommitCommitted(
	ctx context.Context,
	sessionID string,
	runID string,
	segmentID string,
	commitID runtimeidentity.CommitID,
) (bool, error) {
	if segmentID == "" {
		if err := validateWaitingRunCommitIdentity(sessionID, runID, commitID); err != nil {
			return false, err
		}
	} else if err := validateRunCommitIdentity(sessionID, runID, segmentID, commitID); err != nil {
		return false, err
	}
	var found int
	err := conn(ctx, r.db).QueryRowContext(ctx,
		`SELECT count(*)
		   FROM runs
		  WHERE session_id = ? AND run_id = ?
		    AND commit_segment_id = ? AND commit_id = ?
		    AND ((state = ? AND active_segment_id = ?) OR state IN (?, ?))`,
		sessionID,
		runID,
		segmentID,
		commitID.String(),
		runStateRunning.databaseValue(),
		segmentID,
		runStateWaiting.databaseValue(),
		runStateTerminal.databaseValue(),
	).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("sqlite: verify Run commit %q: %w", commitID, err)
	}
	return found == 1, nil
}

func validateRunCommitIdentity(sessionID, runID, segmentID string, commitID runtimeidentity.CommitID) error {
	if err := validateRunCoordinates("verify Run commit", sessionID, runID, segmentID); err != nil {
		return err
	}
	if err := commitID.Validate(); err != nil {
		return fmt.Errorf("sqlite: verify Run commit: %w", err)
	}
	return nil
}

func validateRunCoordinates(operation, sessionID, runID, segmentID string) error {
	if err := validateSessionResource(operation, sessionID); err != nil {
		return err
	}
	if err := validateRunResource(operation, runID); err != nil {
		return err
	}
	if err := validateSegmentResource(operation, segmentID); err != nil {
		return err
	}
	return nil
}

func validateWaitingRunCommitIdentity(sessionID, runID string, commitID runtimeidentity.CommitID) error {
	if err := validateSessionResource("verify waiting Run commit", sessionID); err != nil {
		return err
	}
	if err := validateRunResource("verify waiting Run commit", runID); err != nil {
		return err
	}
	if err := commitID.Validate(); err != nil {
		return fmt.Errorf("sqlite: verify waiting Run commit: %w", err)
	}
	return nil
}
