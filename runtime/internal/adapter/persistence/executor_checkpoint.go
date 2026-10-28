package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

var (
	errExecutorCheckpointRecordNotFound = errors.New("sqlite: executor checkpoint record not found")
	errInvalidExecutorCheckpointRecord  = errors.New("sqlite: invalid executor checkpoint record")
)

// executorCheckpointRecord is the technical record persisted by SQLite.
// Payload remains opaque. Product ownership and recovery policy stay in
// application/agent/runs and are validated again by the consuming adapter.
type executorCheckpointRecord struct {
	ToolResultIDs []transcript.ToolResultID
	Bindings      []plugin.Activation
	Resources     []plugin.Dependency
	RootMemberID  string
	SessionID     string
	Payload       []byte
	BuildID       string
}

func (e executorCheckpointRecord) validate() error {
	if err := transcript.ValidateToolResultReferences(e.ToolResultIDs); err != nil {
		return fmt.Errorf("%w: %w", errInvalidExecutorCheckpointRecord, err)
	}
	if err := plugin.ValidateDependencies(e.Resources); err != nil {
		return fmt.Errorf("%w: %w", errInvalidExecutorCheckpointRecord, err)
	}
	if _, err := checkpointDependencies(e.Bindings, e.Resources); err != nil {
		return err
	}
	if err := runs.ValidateMember(e.RootMemberID); err != nil {
		return fmt.Errorf("%w: %v", errInvalidExecutorCheckpointRecord, err)
	}
	if len(e.Payload) == 0 {
		return fmt.Errorf("%w: payload is empty", errInvalidExecutorCheckpointRecord)
	}
	if _, err := runs.ParseBuild(e.BuildID); err != nil {
		return fmt.Errorf("%w: %v", errInvalidExecutorCheckpointRecord, err)
	}
	if err := identity.ValidateSession(e.SessionID); err != nil {
		return fmt.Errorf("%w: %v", errInvalidExecutorCheckpointRecord, err)
	}
	return nil
}

// ExecutorCheckpointStore persists one opaque checkpoint aggregate per
// executor tree root.
// Payload interpretation and all executor-member topology belong exclusively to
// the execution adapter.
type ExecutorCheckpointStore struct {
	db        *sql.DB
	admission *qimu.Admission
}

// NewExecutorCheckpointStore binds opaque executor checkpoint persistence to a
// database opened via [openDatabase].
func newExecutorCheckpointStore(db *sql.DB) *ExecutorCheckpointStore {
	return &ExecutorCheckpointStore{db: db}
}

// SaveCheckpoint atomically advances one root-owned executor checkpoint. The
// root's Session and build are immutable; only the opaque payload may advance
// between barriers.
func (s *ExecutorCheckpointStore) saveCheckpointRecord(ctx context.Context, checkpoint executorCheckpointRecord) error {
	if err := checkpoint.validate(); err != nil {
		return fmt.Errorf("sqlite: save executor checkpoint: %w", err)
	}
	if s.admission != nil && (len(checkpoint.Bindings) != 0 || len(checkpoint.Resources) != 0) {
		if err := s.admission.CheckAdmitted(ctx); err != nil {
			return err
		}
		if err := validateReferenceWrite(ctx, checkpoint); err != nil {
			return err
		}
	}
	return sqlite.RunInTx(ctx, s.db, func(ctx context.Context) error {
		var owner, buildID string
		err := sqlite.Connection(ctx, s.db).QueryRowContext(ctx,
			`SELECT session_id, build_id
			   FROM executor_checkpoints
			  WHERE root_member_id = ?`,
			checkpoint.RootMemberID,
		).Scan(&owner, &buildID)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = sqlite.Connection(ctx, s.db).ExecContext(ctx,
				`INSERT INTO executor_checkpoints(
					root_member_id, session_id, build_id, payload
				 ) VALUES (?, ?, ?, ?)`,
				checkpoint.RootMemberID,
				checkpoint.SessionID,
				checkpoint.BuildID,
				checkpoint.Payload,
			)
			if err != nil {
				return fmt.Errorf("sqlite: insert executor checkpoint %q: %w", checkpoint.RootMemberID, err)
			}
			return s.replaceReferences(ctx, checkpoint)
		}
		if err != nil {
			return fmt.Errorf("sqlite: inspect executor checkpoint %q before save: %w", checkpoint.RootMemberID, err)
		}
		switch {
		case owner != checkpoint.SessionID:
			return fmt.Errorf(
				"sqlite: executor checkpoint %q belongs to Session %q, not %q: %w",
				checkpoint.RootMemberID,
				owner,
				checkpoint.SessionID,
				errInvalidExecutorCheckpointRecord,
			)
		case buildID != checkpoint.BuildID:
			return fmt.Errorf(
				"sqlite: executor checkpoint %q build is immutable: stored %q, replacement %q: %w",
				checkpoint.RootMemberID,
				buildID,
				checkpoint.BuildID,
				errInvalidExecutorCheckpointRecord,
			)
		}
		result, err := sqlite.Connection(ctx, s.db).ExecContext(ctx,
			`UPDATE executor_checkpoints
			    SET payload = ?
			  WHERE root_member_id = ?`,
			checkpoint.Payload,
			checkpoint.RootMemberID,
		)
		if err != nil {
			return fmt.Errorf("sqlite: advance executor checkpoint %q: %w", checkpoint.RootMemberID, err)
		}
		written, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect advanced executor checkpoint %q: %w", checkpoint.RootMemberID, err)
		}
		if written != 1 {
			return fmt.Errorf("sqlite: advance executor checkpoint %q affected %d rows", checkpoint.RootMemberID, written)
		}
		return s.replaceReferences(ctx, checkpoint)
	})
}

// LoadCheckpoint returns one complete opaque executor checkpoint.
func (s *ExecutorCheckpointStore) loadCheckpointRecord(ctx context.Context, rootMemberID string) (executorCheckpointRecord, error) {
	var checkpoint executorCheckpointRecord
	err := sqlite.RunReadSnapshot(ctx, s.db, func(ctx context.Context) error {
		var err error
		checkpoint, err = s.loadCheckpoint(ctx, rootMemberID)
		return err
	})
	if err != nil {
		return executorCheckpointRecord{}, err
	}
	return checkpoint, nil
}

func (s *ExecutorCheckpointStore) loadSessionCheckpointRecord(ctx context.Context, sessionID string) (executorCheckpointRecord, bool, error) {
	if err := validateSessionResource("load execution context", sessionID); err != nil {
		return executorCheckpointRecord{}, false, err
	}
	var checkpoint executorCheckpointRecord
	found := false
	err := sqlite.RunReadSnapshot(ctx, s.db, func(ctx context.Context) error {
		rows, err := sqlite.Connection(ctx, s.db).QueryContext(ctx, `SELECT root_member_id FROM executor_checkpoints WHERE session_id=? LIMIT 2`, sessionID)
		if err != nil {
			return err
		}
		var roots []string
		for rows.Next() {
			var root string
			if err := rows.Scan(&root); err != nil {
				_ = rows.Close()
				return err
			}
			roots = append(roots, root)
		}
		err = errors.Join(rows.Err(), rows.Close())
		if err != nil {
			return err
		}
		if len(roots) == 0 {
			return nil
		}
		if len(roots) != 1 {
			return fmt.Errorf("%w: Session has multiple execution contexts", errInvalidExecutorCheckpointRecord)
		}
		checkpoint, err = s.loadCheckpoint(ctx, roots[0])
		found = err == nil
		return err
	})
	return checkpoint, found, err
}

// The payload and its body references must be read from the same snapshot.
func (s *ExecutorCheckpointStore) loadCheckpoint(ctx context.Context, rootMemberID string) (executorCheckpointRecord, error) {
	if err := runs.ValidateMember(rootMemberID); err != nil {
		return executorCheckpointRecord{}, fmt.Errorf("sqlite: load executor checkpoint: %w", err)
	}
	var sessionID, buildID string
	var payload []byte
	err := sqlite.Connection(ctx, s.db).QueryRowContext(ctx,
		`SELECT session_id, build_id, payload
		   FROM executor_checkpoints
		  WHERE root_member_id = ?`,
		rootMemberID,
	).Scan(&sessionID, &buildID, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return executorCheckpointRecord{}, fmt.Errorf(
			"sqlite: load executor checkpoint %q: %w",
			rootMemberID,
			errExecutorCheckpointRecordNotFound,
		)
	}
	if err != nil {
		return executorCheckpointRecord{}, fmt.Errorf("sqlite: load executor checkpoint %q: %w", rootMemberID, err)
	}
	checkpoint := executorCheckpointRecord{
		RootMemberID: rootMemberID,
		SessionID:    sessionID,
		Payload:      append([]byte(nil), payload...),
		BuildID:      buildID,
	}
	checkpoint.ToolResultIDs, err = s.toolResultReferences(ctx, rootMemberID)
	if err != nil {
		return executorCheckpointRecord{}, err
	}
	checkpoint.Bindings, checkpoint.Resources, err = s.checkpointReferences(ctx, rootMemberID)
	if err != nil {
		return executorCheckpointRecord{}, err
	}
	if err := checkpoint.validate(); err != nil {
		return executorCheckpointRecord{}, fmt.Errorf("sqlite: load executor checkpoint %q: %w", rootMemberID, err)
	}
	return checkpoint, nil
}

// DeleteCheckpoints removes complete root-owned checkpoint aggregates in one
// transaction, but only when they belong to sessionID. Unknown roots are
// already absent and therefore succeed; a root owned by another Session is
// rejected as corruption rather than deleted.
func (s *ExecutorCheckpointStore) deleteCheckpoints(ctx context.Context, sessionID string, rootIDs []string) error {
	if err := validateSessionResource("delete executor checkpoints", sessionID); err != nil {
		return err
	}
	if len(rootIDs) == 0 {
		return errors.New("sqlite: delete executor checkpoints: no roots")
	}
	seen := make(map[string]struct{}, len(rootIDs))
	for _, rootID := range rootIDs {
		if err := runs.ValidateMember(rootID); err != nil {
			return fmt.Errorf("sqlite: delete executor checkpoints: %w", err)
		}
		if _, duplicate := seen[rootID]; duplicate {
			return fmt.Errorf("sqlite: delete executor checkpoints: duplicate root ID %q", rootID)
		}
		seen[rootID] = struct{}{}
	}
	return sqlite.RunInTx(ctx, s.db, func(ctx context.Context) error {
		for _, rootID := range rootIDs {
			if err := s.deleteOwnedCheckpoint(ctx, sessionID, rootID); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteSessionCheckpoints removes every checkpoint aggregate owned by
// sessionID.
func (s *ExecutorCheckpointStore) deleteSessionCheckpoints(ctx context.Context, sessionID string) error {
	if err := validateSessionResource("delete session executor checkpoints", sessionID); err != nil {
		return err
	}
	return sqlite.RunInTx(ctx, s.db, func(ctx context.Context) error {
		rootIDs, err := s.sessionCheckpointRootIDs(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("sqlite: list executor checkpoints for Session %q: %w", sessionID, err)
		}
		for _, rootID := range rootIDs {
			if err := s.deleteOwnedCheckpoint(ctx, sessionID, rootID); err != nil {
				return err
			}
		}
		_, err = sqlite.Connection(ctx, s.db).ExecContext(ctx, `DELETE FROM execution_trees WHERE session_id = ?`, sessionID)
		return err
	})
}

func (s *ExecutorCheckpointStore) sessionCheckpointRootIDs(
	ctx context.Context,
	sessionID string,
) ([]string, error) {
	rows, err := sqlite.Connection(ctx, s.db).QueryContext(ctx,
		`SELECT root_member_id FROM executor_checkpoints WHERE session_id = ? ORDER BY root_member_id`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	var rootIDs []string
	for rows.Next() {
		var rootID string
		if err := rows.Scan(&rootID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan executor checkpoint root: %w", err)
		}
		rootIDs = append(rootIDs, rootID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close executor checkpoint roots: %w", err)
	}
	return rootIDs, nil
}

func (s *ExecutorCheckpointStore) deleteOwnedCheckpoint(
	ctx context.Context,
	sessionID string,
	rootMemberID string,
) error {
	result, err := sqlite.Connection(ctx, s.db).ExecContext(ctx,
		`DELETE FROM executor_checkpoints WHERE root_member_id = ? AND session_id = ?`,
		rootMemberID,
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: delete executor checkpoint %q for Session %q: %w", rootMemberID, sessionID, err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect deleted executor checkpoint %q: %w", rootMemberID, err)
	}
	if deleted == 1 {
		return nil
	}
	var owner string
	err = sqlite.Connection(ctx, s.db).QueryRowContext(ctx,
		`SELECT session_id FROM executor_checkpoints WHERE root_member_id = ?`,
		rootMemberID,
	).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("sqlite: inspect executor checkpoint %q owner: %w", rootMemberID, err)
	}
	return fmt.Errorf(
		"sqlite: executor checkpoint %q belongs to Session %q, not %q: %w",
		rootMemberID,
		owner,
		sessionID,
		errInvalidExecutorCheckpointRecord,
	)
}
