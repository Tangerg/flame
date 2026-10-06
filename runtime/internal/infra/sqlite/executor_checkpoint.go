package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

var (
	ErrExecutorCheckpointRecordNotFound = errors.New("sqlite: executor checkpoint record not found")
	ErrInvalidExecutorCheckpointRecord  = errors.New("sqlite: invalid executor checkpoint record")
)

// ExecutorCheckpointRecord is the technical record persisted by SQLite.
// Payload remains opaque. Product ownership and recovery policy stay in
// application/agent/runs and are validated again by the consuming adapter.
type ExecutorCheckpointRecord struct {
	ToolResultIDs []toolresult.ID
	Installations []plugin.Dependency
	RootMemberID  string
	SessionID     string
	Payload       []byte
	BuildID       string
	Usage         accounting.Snapshot
}

func (e ExecutorCheckpointRecord) validate() error {
	if err := toolresult.ValidateReferences(e.ToolResultIDs); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExecutorCheckpointRecord, err)
	}
	if err := plugin.ValidateDependencies(e.Installations); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExecutorCheckpointRecord, err)
	}
	if err := runtimeidentity.ValidateMember(e.RootMemberID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidExecutorCheckpointRecord, err)
	}
	if len(e.Payload) == 0 {
		return fmt.Errorf("%w: payload is empty", ErrInvalidExecutorCheckpointRecord)
	}
	if _, err := runtimeidentity.ParseBuild(e.BuildID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidExecutorCheckpointRecord, err)
	}
	if err := resourceid.ValidateSession(e.SessionID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidExecutorCheckpointRecord, err)
	}
	if err := e.Usage.Validate(); err != nil {
		return fmt.Errorf("%w: usage: %w", ErrInvalidExecutorCheckpointRecord, err)
	}
	return nil
}

// ExecutorCheckpointStore persists one opaque checkpoint aggregate per
// executor tree root.
// Payload interpretation and all executor-member topology belong exclusively to
// the execution adapter.
type ExecutorCheckpointStore struct {
	db *sql.DB
}

// NewExecutorCheckpointStore binds opaque executor checkpoint persistence to a
// database opened via [Open].
func NewExecutorCheckpointStore(db *sql.DB) *ExecutorCheckpointStore {
	return &ExecutorCheckpointStore{db: db}
}

type executorUsageWire struct {
	Models []executorModelUsageWire `json:"models"`
}

type executorModelUsageWire struct {
	Model            string   `json:"model"`
	InputTokens      int64    `json:"input_tokens"`
	OutputTokens     int64    `json:"output_tokens"`
	ReasoningTokens  int64    `json:"reasoning_tokens"`
	CacheReadTokens  int64    `json:"cache_read_tokens"`
	CacheWriteTokens int64    `json:"cache_write_tokens"`
	CostUSD          *float64 `json:"cost_usd,omitzero"`
	Calls            int      `json:"calls"`
}

// SaveCheckpoint atomically advances one root-owned executor checkpoint. The
// root's Session and build are immutable; only the opaque payload and
// cumulative usage may advance between barriers.
func (e *ExecutorCheckpointStore) SaveCheckpoint(ctx context.Context, checkpoint ExecutorCheckpointRecord) error {
	if err := checkpoint.validate(); err != nil {
		return fmt.Errorf("sqlite: save executor checkpoint: %w", err)
	}
	encodedUsage, err := encodeExecutorUsage(checkpoint.Usage)
	if err != nil {
		return fmt.Errorf("sqlite: encode executor checkpoint usage: %w", err)
	}
	return RunInTx(ctx, e.db, func(ctx context.Context) error {
		var owner, buildID, storedUsageData string
		err := conn(ctx, e.db).QueryRowContext(ctx,
			`SELECT session_id, build_id, usage
			   FROM executor_checkpoints
			  WHERE root_member_id = ?`,
			checkpoint.RootMemberID,
		).Scan(&owner, &buildID, &storedUsageData)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = conn(ctx, e.db).ExecContext(ctx,
				`INSERT INTO executor_checkpoints(
					root_member_id, session_id, build_id, payload, usage
				 ) VALUES (?, ?, ?, ?, ?)`,
				checkpoint.RootMemberID,
				checkpoint.SessionID,
				checkpoint.BuildID,
				checkpoint.Payload,
				string(encodedUsage),
			)
			if err != nil {
				return fmt.Errorf("sqlite: insert executor checkpoint %q: %w", checkpoint.RootMemberID, err)
			}
			return e.replaceReferences(ctx, checkpoint)
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
				ErrInvalidExecutorCheckpointRecord,
			)
		case buildID != checkpoint.BuildID:
			return fmt.Errorf(
				"sqlite: executor checkpoint %q build is immutable: stored %q, replacement %q: %w",
				checkpoint.RootMemberID,
				buildID,
				checkpoint.BuildID,
				ErrInvalidExecutorCheckpointRecord,
			)
		}
		storedUsage, err := decodeExecutorUsage(storedUsageData)
		if err != nil {
			return fmt.Errorf(
				"sqlite: decode stored executor checkpoint %q usage: %w: %w",
				checkpoint.RootMemberID,
				ErrInvalidExecutorCheckpointRecord,
				err,
			)
		}
		if validateAdvanceFromErr := checkpoint.Usage.ValidateAdvanceFrom(storedUsage); validateAdvanceFromErr != nil {
			return fmt.Errorf(
				"sqlite: executor checkpoint %q cumulative usage cannot advance: %w: %w",
				checkpoint.RootMemberID,
				ErrInvalidExecutorCheckpointRecord,
				validateAdvanceFromErr,
			)
		}
		result, err := conn(ctx, e.db).ExecContext(ctx,
			`UPDATE executor_checkpoints
			    SET payload = ?, usage = ?
			  WHERE root_member_id = ?`,
			checkpoint.Payload,
			string(encodedUsage),
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
		return e.replaceReferences(ctx, checkpoint)
	})
}

// LoadCheckpoint returns one complete opaque executor checkpoint.
func (e *ExecutorCheckpointStore) LoadCheckpoint(ctx context.Context, rootMemberID string) (ExecutorCheckpointRecord, error) {
	var checkpoint ExecutorCheckpointRecord
	err := RunInTx(ctx, e.db, func(ctx context.Context) error {
		var err error
		checkpoint, err = e.loadCheckpoint(ctx, rootMemberID)
		return err
	})
	if err != nil {
		return ExecutorCheckpointRecord{}, err
	}
	return checkpoint, nil
}

// The payload and its body references must be read from the same snapshot.
func (e *ExecutorCheckpointStore) loadCheckpoint(ctx context.Context, rootMemberID string) (ExecutorCheckpointRecord, error) {
	if err := runtimeidentity.ValidateMember(rootMemberID); err != nil {
		return ExecutorCheckpointRecord{}, fmt.Errorf("sqlite: load executor checkpoint: %w", err)
	}
	var sessionID, buildID, usageData string
	var payload []byte
	err := conn(ctx, e.db).QueryRowContext(ctx,
		`SELECT session_id, build_id, payload, usage
		   FROM executor_checkpoints
		  WHERE root_member_id = ?`,
		rootMemberID,
	).Scan(&sessionID, &buildID, &payload, &usageData)
	if errors.Is(err, sql.ErrNoRows) {
		return ExecutorCheckpointRecord{}, fmt.Errorf(
			"sqlite: load executor checkpoint %q: %w",
			rootMemberID,
			ErrExecutorCheckpointRecordNotFound,
		)
	}
	if err != nil {
		return ExecutorCheckpointRecord{}, fmt.Errorf("sqlite: load executor checkpoint %q: %w", rootMemberID, err)
	}
	usage, err := decodeExecutorUsage(usageData)
	if err != nil {
		return ExecutorCheckpointRecord{}, fmt.Errorf(
			"sqlite: decode executor checkpoint %q usage: %w: %w",
			rootMemberID,
			ErrInvalidExecutorCheckpointRecord,
			err,
		)
	}
	checkpoint := ExecutorCheckpointRecord{
		RootMemberID: rootMemberID,
		SessionID:    sessionID,
		Payload:      append([]byte(nil), payload...),
		BuildID:      buildID,
		Usage:        usage,
	}
	checkpoint.ToolResultIDs, err = e.toolResultReferences(ctx, rootMemberID)
	if err != nil {
		return ExecutorCheckpointRecord{}, err
	}
	checkpoint.Installations, err = e.installationDependencies(ctx, rootMemberID)
	if err != nil {
		return ExecutorCheckpointRecord{}, err
	}
	if err := checkpoint.validate(); err != nil {
		return ExecutorCheckpointRecord{}, fmt.Errorf("sqlite: load executor checkpoint %q: %w", rootMemberID, err)
	}
	return checkpoint, nil
}

func encodeExecutorUsage(usage accounting.Snapshot) ([]byte, error) {
	wire := executorUsageWire{Models: make([]executorModelUsageWire, len(usage.Models))}
	for index, model := range usage.Models {
		wire.Models[index] = executorModelUsageWire{
			Model:            model.Model,
			InputTokens:      model.InputTokens,
			OutputTokens:     model.OutputTokens,
			ReasoningTokens:  model.ReasoningTokens,
			CacheReadTokens:  model.CacheReadTokens,
			CacheWriteTokens: model.CacheWriteTokens,
			CostUSD:          model.Cost.OptionalUSD(),
			Calls:            model.Calls,
		}
	}
	return encodeStoredJSON(wire)
}

func decodeExecutorUsage(data string) (accounting.Snapshot, error) {
	var wire executorUsageWire
	if err := decodeStoredJSON([]byte(data), &wire); err != nil {
		return accounting.Snapshot{}, err
	}
	if wire.Models == nil {
		return accounting.Snapshot{}, errors.New("usage models must be an array")
	}
	usage := accounting.Snapshot{Models: make([]accounting.ModelUsage, len(wire.Models))}
	for index, model := range wire.Models {
		cost, err := accounting.CostFromOptional(model.CostUSD)
		if err != nil {
			return accounting.Snapshot{}, fmt.Errorf("usage model[%d] cost: %w", index, err)
		}
		usage.Models[index] = accounting.ModelUsage{
			Model: model.Model,
			Tokens: accounting.Tokens{
				InputTokens:      model.InputTokens,
				OutputTokens:     model.OutputTokens,
				ReasoningTokens:  model.ReasoningTokens,
				CacheReadTokens:  model.CacheReadTokens,
				CacheWriteTokens: model.CacheWriteTokens,
			},
			Cost:  cost,
			Calls: model.Calls,
		}
	}
	if err := usage.Validate(); err != nil {
		return accounting.Snapshot{}, err
	}
	return usage, nil
}

// DeleteCheckpoints removes complete root-owned checkpoint aggregates in one
// transaction, but only when they belong to sessionID. Unknown roots are
// already absent and therefore succeed; a root owned by another Session is
// rejected as corruption rather than deleted.
func (e *ExecutorCheckpointStore) DeleteCheckpoints(ctx context.Context, sessionID string, rootIDs []string) error {
	if err := validateSessionResource("delete executor checkpoints", sessionID); err != nil {
		return err
	}
	if len(rootIDs) == 0 {
		return errors.New("sqlite: delete executor checkpoints: no roots")
	}
	seen := make(map[string]struct{}, len(rootIDs))
	for _, rootID := range rootIDs {
		if err := runtimeidentity.ValidateMember(rootID); err != nil {
			return fmt.Errorf("sqlite: delete executor checkpoints: %w", err)
		}
		if _, duplicate := seen[rootID]; duplicate {
			return fmt.Errorf("sqlite: delete executor checkpoints: duplicate root ID %q", rootID)
		}
		seen[rootID] = struct{}{}
	}
	return RunInTx(ctx, e.db, func(ctx context.Context) error {
		for _, rootID := range rootIDs {
			if err := e.deleteOwnedCheckpoint(ctx, sessionID, rootID); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteSessionCheckpoints removes every checkpoint aggregate owned by
// sessionID.
func (e *ExecutorCheckpointStore) DeleteSessionCheckpoints(ctx context.Context, sessionID string) error {
	if err := validateSessionResource("delete session executor checkpoints", sessionID); err != nil {
		return err
	}
	return RunInTx(ctx, e.db, func(ctx context.Context) error {
		rootIDs, err := e.sessionCheckpointRootIDs(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("sqlite: list executor checkpoints for Session %q: %w", sessionID, err)
		}
		for _, rootID := range rootIDs {
			if err := e.deleteOwnedCheckpoint(ctx, sessionID, rootID); err != nil {
				return err
			}
		}
		_, err = conn(ctx, e.db).ExecContext(ctx, `DELETE FROM execution_trees WHERE session_id = ?`, sessionID)
		return err
	})
}

func (e *ExecutorCheckpointStore) sessionCheckpointRootIDs(
	ctx context.Context,
	sessionID string,
) ([]string, error) {
	rows, err := conn(ctx, e.db).QueryContext(ctx,
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

func (e *ExecutorCheckpointStore) deleteOwnedCheckpoint(
	ctx context.Context,
	sessionID string,
	rootMemberID string,
) error {
	result, err := conn(ctx, e.db).ExecContext(ctx,
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
	err = conn(ctx, e.db).QueryRowContext(ctx,
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
		ErrInvalidExecutorCheckpointRecord,
	)
}
