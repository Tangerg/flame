package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrWorkspaceMutationPending reports that an unfinished rollback still owns the
// Session or the working tree a new rollback addresses.
var ErrWorkspaceMutationPending = errors.New("sqlite: workspace mutation is pending")

// WorkspaceMutationRecord is the whole identity of one recoverable file
// rollback: recovery re-drives exactly this operation, so every field
// participates in the identity Complete matches.
type WorkspaceMutationRecord struct {
	SessionID      string
	CWD            string
	ToRunID        string
	RestoreHistory bool
}

// WorkspaceMutationStore is the recoverable operation log for file rollbacks,
// backed by the pending_workspace_mutations table. Unlike the write-set
// stores, its writes deliberately do NOT join an ambient transaction (they use
// the *sql.DB directly, never conn(ctx)): the intent must commit on its own
// before the working tree is touched, and the completion on its own after the
// requested effects commit. The row protects a non-atomic multi-path Git reset
// and, when requested, the separate SQLite history transaction.
//
// A logged intent is the ONLY record that a reset may have changed part of a
// tree, so it owns recovery for that tree until it completes: Record refuses to
// displace a different pending operation and Complete clears only the operation
// it is given.
//
// Safe for concurrent use; the *sql.DB serializes writes (MaxOpenConns 1, see
// [Open]).
type WorkspaceMutationStore struct {
	db *sql.DB
}

// NewWorkspaceMutationStore wires a database opened via [Open] to the
// operation-log surface.
func NewWorkspaceMutationStore(db *sql.DB) *WorkspaceMutationStore {
	return &WorkspaceMutationStore{db: db}
}

// Record logs a rollback's intent before the working tree is touched.
// Re-logging the same operation is a no-op so an interrupted rollback can
// re-drive its own intent; any other pending operation on this Session or tree
// is [ErrWorkspaceMutationPending] rather than replaced.
func (w *WorkspaceMutationStore) Record(ctx context.Context, m WorkspaceMutationRecord) error {
	if err := validateSessionResource("record workspace mutation", m.SessionID); err != nil {
		return err
	}
	if err := validateRunResource("record workspace mutation", m.ToRunID); err != nil {
		return err
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: record workspace mutation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	recorded, err := ownsPendingWorkspaceMutation(ctx, tx, m)
	if err != nil {
		return err
	}
	if recorded {
		return nil
	}
	result, err := tx.ExecContext(ctx,
		`INSERT INTO pending_workspace_mutations(session_id, cwd, to_run_id, restore_history)
		 SELECT sessions.id, sessions.workspace_path, runs.run_id, ?
		   FROM sessions
		   JOIN runs ON runs.run_id = ? AND runs.session_id = sessions.id
		  WHERE sessions.id = ? AND sessions.workspace_path = ?`,
		m.RestoreHistory,
		m.ToRunID,
		m.SessionID,
		m.CWD)
	if err != nil {
		return fmt.Errorf("sqlite: record workspace mutation: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect workspace mutation record: %w", err)
	}
	if changed != 1 {
		return errors.New("sqlite: workspace mutation has no matching Run boundary")
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit workspace mutation record: %w", err)
	}
	return nil
}

func ownsPendingWorkspaceMutation(ctx context.Context, tx *sql.Tx, m WorkspaceMutationRecord) (bool, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT session_id, cwd, to_run_id, restore_history
		   FROM pending_workspace_mutations
		  WHERE session_id = ? OR cwd = ?`,
		m.SessionID,
		m.CWD)
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect pending workspace mutations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	recorded := false
	for rows.Next() {
		var existing WorkspaceMutationRecord
		if err := rows.Scan(&existing.SessionID, &existing.CWD, &existing.ToRunID, &existing.RestoreHistory); err != nil {
			return false, fmt.Errorf("sqlite: scan pending workspace mutation: %w", err)
		}
		if existing != m {
			return false, ErrWorkspaceMutationPending
		}
		recorded = true
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("sqlite: iterate pending workspace mutations: %w", err)
	}
	return recorded, nil
}

// Complete clears the logged intent of exactly the operation m names, once its
// file restore and, when requested, durable truncation have committed. Matching
// the whole identity keeps a later rollback from clearing a predecessor's
// unfinished record. Idempotent: deleting an absent row is not an error.
func (w *WorkspaceMutationStore) Complete(ctx context.Context, m WorkspaceMutationRecord) error {
	if err := validateSessionResource("complete workspace mutation", m.SessionID); err != nil {
		return err
	}
	if err := validateRunResource("complete workspace mutation", m.ToRunID); err != nil {
		return err
	}
	_, err := w.db.ExecContext(ctx,
		`DELETE FROM pending_workspace_mutations
		  WHERE session_id = ? AND cwd = ? AND to_run_id = ? AND restore_history = ?`,
		m.SessionID, m.CWD, m.ToRunID, m.RestoreHistory)
	if err != nil {
		return fmt.Errorf("sqlite: complete workspace mutation: %w", err)
	}
	return nil
}

// ListPending returns every rollback a crash left unfinished, oldest first, for
// boot recovery to re-drive.
func (w *WorkspaceMutationStore) ListPending(ctx context.Context) ([]WorkspaceMutationRecord, error) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT session_id, cwd, to_run_id, restore_history FROM pending_workspace_mutations ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list workspace mutations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []WorkspaceMutationRecord
	for rows.Next() {
		var m WorkspaceMutationRecord
		if err := rows.Scan(&m.SessionID, &m.CWD, &m.ToRunID, &m.RestoreHistory); err != nil {
			return nil, fmt.Errorf("sqlite: scan workspace mutation: %w", err)
		}
		if err := validateSessionResource("restore workspace mutation", m.SessionID); err != nil {
			return nil, err
		}
		if err := validateRunResource("restore workspace mutation", m.ToRunID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate workspace mutations: %w", err)
	}
	return out, nil
}
