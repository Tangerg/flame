package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/qimu/sqlite"
)

// errWorkspaceMutationPending reports that an unfinished rollback still owns the
// Session or the working tree a new rollback addresses.
var errWorkspaceMutationPending = errors.New("sqlite: workspace mutation is pending")

// workspaceMutationRecord is the whole identity of one recoverable file
// rollback: recovery re-drives exactly this operation, so every field
// participates in the identity Complete matches.
type workspaceMutationRecord struct {
	SessionID      string
	CWD            string
	ToRunID        string
	RestoreHistory bool
}

// WorkspaceMutationStore is the recoverable operation log for file rollbacks,
// backed by the pending_workspace_mutations table. Unlike the write-set
// stores, its writes deliberately do NOT join an ambient transaction (they use
// the *sql.DB directly, never sqlitebase.Connection(ctx)): the intent must commit on its own
// before the working tree is touched, and the completion on its own after the
// requested effects commit. The row protects a non-atomic multi-path Git checkout
// and, when requested, the separate SQLite history transaction.
//
// A logged intent is the ONLY record that a checkout may have changed part of a
// tree, so it owns recovery for that tree until it completes: Record refuses to
// displace a different pending operation and Complete clears only the operation
// it is given.
//
// Safe for concurrent use; the *sql.DB serializes writes (MaxOpenConns 1, see
// [Open]).
type WorkspaceMutationStore struct {
	db *sql.DB
}

// NewWorkspaceMutationStore wires a database opened via [openDatabase] to the
// operation-log surface.
func newWorkspaceMutationStore(db *sql.DB) *WorkspaceMutationStore {
	return &WorkspaceMutationStore{db: db}
}

// Record logs a rollback's intent before the working tree is touched.
// Re-logging the same operation is a no-op so an interrupted rollback can
// re-drive its own intent; any other pending operation on this Session or tree
// is [errWorkspaceMutationPending] rather than replaced. The boolean reports
// whether this call created the intent; an adopted intent may already have
// changed files and cannot be retired as a pre-checkout refusal.
func (s *WorkspaceMutationStore) recordMutation(ctx context.Context, m workspaceMutationRecord) (bool, error) {
	if err := validateSessionResource("record workspace mutation", m.SessionID); err != nil {
		return false, err
	}
	if err := validateRunResource("record workspace mutation", m.ToRunID); err != nil {
		return false, err
	}
	created := false
	err := sqlite.RunIndependentTx(ctx, s.db, func(ctx context.Context) error {
		tx := sqlite.Connection(ctx, s.db)
		recorded, err := ownsPendingWorkspaceMutation(ctx, tx, m)
		if err != nil || recorded {
			return err
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
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return errors.New("sqlite: workspace mutation has no matching Run boundary")
		}
		created = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("sqlite: record workspace mutation: %w", err)
	}
	return created, nil
}

func ownsPendingWorkspaceMutation(ctx context.Context, tx sqlite.Executor, m workspaceMutationRecord) (bool, error) {
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
		var existing workspaceMutationRecord
		if err := rows.Scan(&existing.SessionID, &existing.CWD, &existing.ToRunID, &existing.RestoreHistory); err != nil {
			return false, fmt.Errorf("sqlite: scan pending workspace mutation: %w", err)
		}
		if existing != m {
			return false, errWorkspaceMutationPending
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
func (s *WorkspaceMutationStore) completeMutation(ctx context.Context, m workspaceMutationRecord) error {
	if err := validateSessionResource("complete workspace mutation", m.SessionID); err != nil {
		return err
	}
	if err := validateRunResource("complete workspace mutation", m.ToRunID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM pending_workspace_mutations
		  WHERE session_id = ? AND cwd = ? AND to_run_id = ? AND restore_history = ?`,
		m.SessionID, m.CWD, m.ToRunID, m.RestoreHistory)
	if err != nil {
		return fmt.Errorf("sqlite: complete workspace mutation: %w", err)
	}
	return nil
}

// ListPending returns every rollback a crash left unfinished for boot recovery
// to re-drive. Record admits at most one pending operation per Session and per
// working tree, so no two rows here can touch the same tree and any total order
// re-drives them correctly; Session identity supplies a stable one.
func (s *WorkspaceMutationStore) pendingMutations(ctx context.Context) ([]workspaceMutationRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, cwd, to_run_id, restore_history FROM pending_workspace_mutations ORDER BY session_id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list workspace mutations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []workspaceMutationRecord
	for rows.Next() {
		var m workspaceMutationRecord
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
