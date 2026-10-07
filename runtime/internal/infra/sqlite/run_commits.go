package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/scope/core/chat"
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

// ResultPublicationCommitted proves exact content under the currently active
// Segment. A stale writer cannot adopt a receipt from a replaced Segment.
func (r *RunStore) ResultPublicationCommitted(ctx context.Context, sessionID, runID, segmentID, publicationID, digest string) (bool, error) {
	if err := r.RequireActiveSegment(ctx, sessionID, runID, segmentID); err != nil {
		return false, err
	}
	var storedSession, storedRun, storedSegment, storedDigest string
	err := conn(ctx, r.db).QueryRowContext(ctx, `SELECT session_id, run_id, segment_id, digest FROM result_publications WHERE publication_id = ?`, publicationID).Scan(&storedSession, &storedRun, &storedSegment, &storedDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sqlite: read result publication: %w", err)
	}
	if storedSession != sessionID || storedRun != runID || storedSegment != segmentID || storedDigest != digest {
		return false, errors.New("sqlite: result publication identity conflicts with stored content or owner")
	}
	return true, nil
}

// RecordResultPublication participates in the transaction owning its results.
func (r *RunStore) RecordResultPublication(ctx context.Context, sessionID, runID, segmentID, publicationID, digest string, results []chat.ToolResult) error {
	if err := r.RequireActiveSegment(ctx, sessionID, runID, segmentID); err != nil {
		return err
	}
	if err := runtimeidentity.ValidateEffect(publicationID); err != nil {
		return err
	}
	if digest == "" {
		return errors.New("sqlite: result publication digest is required")
	}

	for _, result := range results {
		if err := result.Validate(); err != nil {
			return fmt.Errorf("sqlite: exact tool result: %w", err)
		}
	}
	encoded, err := encodeStoredJSON(results)
	if err != nil {
		return fmt.Errorf("sqlite: encode exact tool results: %w", err)
	}
	_, err = conn(ctx, r.db).ExecContext(ctx, `INSERT INTO result_publications(publication_id, digest, session_id, run_id, segment_id, source_message_seq, model_results)
 VALUES (?, ?, ?, ?, ?, (SELECT max(seq) FROM messages WHERE conversation_id = ?), ?)`, publicationID, digest, sessionID, runID, segmentID, sessionID, string(encoded))
	if err != nil {
		return fmt.Errorf("sqlite: record result publication: %w", err)
	}
	return nil
}

func (r *RunStore) UnpublishedToolResults(ctx context.Context, sessionID, runID string) ([]chat.ToolResult, error) {
	rows, err := conn(ctx, r.db).QueryContext(ctx, `SELECT model_results FROM result_publications
 WHERE session_id = ? AND run_id = ? AND (model_results IS NULL OR source_message_seq = (SELECT max(seq) FROM messages WHERE conversation_id = ?))
 ORDER BY publication_id`, sessionID, runID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: read unpublished tool results: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var results []chat.ToolResult
	for rows.Next() {
		var encoded sql.NullString
		if err := rows.Scan(&encoded); err != nil {
			return nil, fmt.Errorf("sqlite: scan unpublished tool results: %w", err)
		}
		if !encoded.Valid {
			return nil, errors.New("sqlite: run has publication receipts without exact tool result evidence; terminal conversation cannot be reconstructed")
		}
		var batch []chat.ToolResult
		if err := decodeStoredJSON([]byte(encoded.String), &batch); err != nil {
			return nil, fmt.Errorf("sqlite: decode unpublished tool results: %w", err)
		}
		for _, result := range batch {
			if err := result.Validate(); err != nil {
				return nil, fmt.Errorf("sqlite: stored unpublished tool result: %w", err)
			}
		}
		results = append(results, batch...)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate unpublished tool results: %w", err)
	}
	return results, nil
}
