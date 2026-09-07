package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

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

type executorScopeWire struct {
	SessionID         string `json:"session_id"`
	CWD               string `json:"cwd"`
	WorkspaceCWD      string `json:"workspace_cwd"`
	Isolated          bool   `json:"isolated"`
	GoalIncarnationID string `json:"goal_incarnation_id"`
}

type executorLimitsWire struct {
	Type           runLimitKind `json:"type"`
	MaxTotalTokens *int64       `json:"max_total_tokens,omitempty"`
	MaxBudgetUSD   *float64     `json:"max_budget_usd,omitempty"`
	MaxSteps       *int         `json:"max_steps,omitempty"`
}

type executorCapabilitiesWire struct {
	ChildRuns      bool     `json:"child_runs"`
	InterruptKinds []string `json:"interrupt_kinds"`
}

type executorPolicyWire struct {
	Scope           executorScopeWire         `json:"scope"`
	Provider        string                    `json:"provider"`
	Model           string                    `json:"model"`
	ReasoningEffort string                    `json:"reasoning_effort"`
	Limits          executorLimitsWire        `json:"limits"`
	Capabilities    *executorCapabilitiesWire `json:"capabilities"`
}

type executorModelUsageWire struct {
	Model            string   `json:"model"`
	PromptTokens     int64    `json:"prompt_tokens"`
	CompletionTokens int64    `json:"completion_tokens"`
	ReasoningTokens  int64    `json:"reasoning_tokens"`
	CacheReadTokens  int64    `json:"cache_read_tokens"`
	CacheWriteTokens int64    `json:"cache_write_tokens"`
	CostUSD          *float64 `json:"cost_usd,omitempty"`
	Calls            int      `json:"calls"`
}

// SaveCheckpoint atomically advances one root-owned executor checkpoint. The
// root's Session, build, host scope, model selection, and budget are immutable;
// only the opaque payload and cumulative usage may advance between barriers.
func (e *ExecutorCheckpointStore) SaveCheckpoint(ctx context.Context, checkpoint run.Checkpoint) error {
	if checkpoint.IsZero() {
		return fmt.Errorf("sqlite: save executor checkpoint: %w", run.ErrInvalidCheckpoint)
	}
	state := checkpoint.State()
	encodedPolicy, err := encodeExecutorPolicy(state)
	if err != nil {
		return fmt.Errorf("sqlite: encode executor checkpoint policy: %w", err)
	}
	encodedUsage, err := encodeExecutorUsage(state.Usage)
	if err != nil {
		return fmt.Errorf("sqlite: encode executor checkpoint usage: %w", err)
	}
	return RunInTx(ctx, e.db, func(ctx context.Context) error {
		current, err := e.LoadCheckpoint(ctx, state.RootMemberID)
		if errors.Is(err, run.ErrCheckpointNotFound) {
			_, err = conn(ctx, e.db).ExecContext(ctx,
				`INSERT INTO executor_checkpoints(root_member_id, session_id, build_id, payload, policy, usage) VALUES (?, ?, ?, ?, ?, ?)`,
				state.RootMemberID, state.Scope.SessionID, state.BuildID, state.Payload, string(encodedPolicy), string(encodedUsage))
			if err != nil {
				return fmt.Errorf("sqlite: insert executor checkpoint %q: %w", state.RootMemberID, err)
			}
			return nil
		}
		if err != nil {
			return err
		}
		if err := current.ValidateSuccessor(checkpoint); err != nil {
			return fmt.Errorf("sqlite: advance executor checkpoint %q: %w", state.RootMemberID, err)
		}
		result, err := conn(ctx, e.db).ExecContext(ctx,
			`UPDATE executor_checkpoints SET payload = ?, usage = ? WHERE root_member_id = ?`, state.Payload, string(encodedUsage), state.RootMemberID)
		if err != nil {
			return fmt.Errorf("sqlite: advance executor checkpoint %q: %w", state.RootMemberID, err)
		}
		written, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect advanced executor checkpoint %q: %w", state.RootMemberID, err)
		}
		if written != 1 {
			return fmt.Errorf("sqlite: advance executor checkpoint %q affected %d rows", state.RootMemberID, written)
		}
		return nil
	})
}

// LoadCheckpoint returns one complete opaque executor checkpoint.
func (e *ExecutorCheckpointStore) LoadCheckpoint(ctx context.Context, rootMemberID string) (run.Checkpoint, error) {
	if _, err := runtimeidentity.ParseMember(rootMemberID); err != nil {
		return run.Checkpoint{}, fmt.Errorf("sqlite: load executor checkpoint: %w", err)
	}
	var owner, buildID, policyData, usageData string
	var payload []byte
	err := conn(ctx, e.db).QueryRowContext(ctx,
		`SELECT session_id, build_id, payload, policy, usage
		   FROM executor_checkpoints
		  WHERE root_member_id = ?`,
		rootMemberID,
	).Scan(&owner, &buildID, &payload, &policyData, &usageData)
	if errors.Is(err, sql.ErrNoRows) {
		return run.Checkpoint{}, fmt.Errorf(
			"sqlite: load executor checkpoint %q: %w",
			rootMemberID,
			run.ErrCheckpointNotFound,
		)
	}
	if err != nil {
		return run.Checkpoint{}, fmt.Errorf("sqlite: load executor checkpoint %q: %w", rootMemberID, err)
	}
	policy, err := decodeExecutorPolicy(policyData)
	if err != nil {
		return run.Checkpoint{}, fmt.Errorf(
			"sqlite: decode executor checkpoint %q policy: %w: %w",
			rootMemberID,
			run.ErrInvalidCheckpoint,
			err,
		)
	}
	usage, err := decodeExecutorUsage(usageData)
	if err != nil {
		return run.Checkpoint{}, fmt.Errorf(
			"sqlite: decode executor checkpoint %q usage: %w: %w",
			rootMemberID,
			run.ErrInvalidCheckpoint,
			err,
		)
	}
	checkpoint := policy
	checkpoint.RootMemberID = rootMemberID
	checkpoint.Payload = payload
	checkpoint.BuildID = buildID
	checkpoint.Usage = usage
	if checkpoint.Scope.SessionID != owner {
		return run.Checkpoint{}, fmt.Errorf("sqlite: checkpoint owner differs from policy: %w", run.ErrInvalidCheckpoint)
	}
	restored, err := run.NewCheckpoint(checkpoint)
	if err != nil {
		return run.Checkpoint{}, fmt.Errorf("sqlite: load executor checkpoint %q: %w", rootMemberID, err)
	}
	return restored, nil
}

func encodeExecutorPolicy(checkpoint run.CheckpointState) ([]byte, error) {
	var interruptKinds []string
	if checkpoint.Capabilities.InterruptKinds != nil {
		interruptKinds = make([]string, len(checkpoint.Capabilities.InterruptKinds))
	}
	for index, kind := range checkpoint.Capabilities.InterruptKinds {
		interruptKinds[index] = kind.String()
	}
	limits := runLimitsRowOf(checkpoint.Limits)
	return json.Marshal(executorPolicyWire{
		Scope: executorScopeWire{
			SessionID:         checkpoint.Scope.SessionID,
			CWD:               checkpoint.Scope.CWD,
			WorkspaceCWD:      checkpoint.Scope.WorkspaceCWD,
			Isolated:          checkpoint.Scope.Isolated,
			GoalIncarnationID: checkpoint.Scope.GoalIncarnationID,
		},
		Provider:        checkpoint.ModelSelection.Provider(),
		Model:           checkpoint.ModelSelection.Model(),
		ReasoningEffort: checkpoint.ModelSelection.ReasoningEffort(),
		Limits: executorLimitsWire{
			Type: limits.Type, MaxTotalTokens: limits.MaxTotalTokens,
			MaxBudgetUSD: limits.MaxBudgetUSD, MaxSteps: limits.MaxSteps,
		},
		Capabilities: &executorCapabilitiesWire{
			ChildRuns:      checkpoint.Capabilities.ChildRuns,
			InterruptKinds: interruptKinds,
		},
	})
}

func decodeExecutorPolicy(data string) (run.CheckpointState, error) {
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire executorPolicyWire
	if err := decoder.Decode(&wire); err != nil {
		return run.CheckpointState{}, err
	}
	if wire.Capabilities == nil {
		return run.CheckpointState{}, errors.New("policy capabilities are required")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return run.CheckpointState{}, errors.New("policy has a trailing JSON value")
		}
		return run.CheckpointState{}, fmt.Errorf("policy trailing JSON: %w", err)
	}
	scope := run.ExecutionScope{
		SessionID:         wire.Scope.SessionID,
		CWD:               wire.Scope.CWD,
		WorkspaceCWD:      wire.Scope.WorkspaceCWD,
		Isolated:          wire.Scope.Isolated,
		GoalIncarnationID: wire.Scope.GoalIncarnationID,
	}
	limits, err := runLimitsFromStored(
		wire.Limits.Type, wire.Limits.MaxTotalTokens, wire.Limits.MaxSteps, wire.Limits.MaxBudgetUSD,
	)
	if err != nil {
		return run.CheckpointState{}, err
	}
	capabilities := run.Capabilities{
		ChildRuns: wire.Capabilities.ChildRuns,
	}
	if wire.Capabilities.InterruptKinds != nil {
		capabilities.InterruptKinds = make([]interrupt.Kind, len(wire.Capabilities.InterruptKinds))
	}
	for index, value := range wire.Capabilities.InterruptKinds {
		kind, ok := interrupt.ParseKind(value)
		if !ok {
			return run.CheckpointState{}, fmt.Errorf(
				"policy capability interrupt kind[%d] %q is unknown",
				index,
				value,
			)
		}
		capabilities.InterruptKinds[index] = kind
	}
	selection, err := modelref.NewWithReasoningEffort(wire.Provider, wire.Model, wire.ReasoningEffort)
	if err != nil {
		return run.CheckpointState{}, fmt.Errorf("policy model selection: %w", err)
	}
	return run.CheckpointState{
		Scope:          scope,
		ModelSelection: selection,
		Limits:         limits,
		Capabilities:   capabilities,
	}, nil
}

func encodeExecutorUsage(usage accounting.Snapshot) ([]byte, error) {
	wire := executorUsageWire{Models: make([]executorModelUsageWire, len(usage.Models))}
	for index, model := range usage.Models {
		wire.Models[index] = executorModelUsageWire{
			Model:            model.Model,
			PromptTokens:     model.PromptTokens,
			CompletionTokens: model.CompletionTokens,
			ReasoningTokens:  model.ReasoningTokens,
			CacheReadTokens:  model.CacheReadTokens,
			CacheWriteTokens: model.CacheWriteTokens,
			CostUSD:          model.Cost.OptionalUSD(),
			Calls:            model.Calls,
		}
	}
	return json.Marshal(wire)
}

func decodeExecutorUsage(data string) (accounting.Snapshot, error) {
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire executorUsageWire
	if err := decoder.Decode(&wire); err != nil {
		return accounting.Snapshot{}, err
	}
	if wire.Models == nil {
		return accounting.Snapshot{}, errors.New("usage models must be an array")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return accounting.Snapshot{}, errors.New("usage has a trailing JSON value")
		}
		return accounting.Snapshot{}, fmt.Errorf("usage trailing JSON: %w", err)
	}
	usage := accounting.Snapshot{Models: make([]accounting.ModelUsage, len(wire.Models))}
	for index, model := range wire.Models {
		cost, err := accounting.CostFromOptional(model.CostUSD)
		if err != nil {
			return accounting.Snapshot{}, fmt.Errorf("usage model[%d] cost: %w", index, err)
		}
		usage.Models[index] = accounting.ModelUsage{
			Model: model.Model,
			TokenUsage: accounting.TokenUsage{
				PromptTokens:     model.PromptTokens,
				CompletionTokens: model.CompletionTokens,
				ReasoningTokens:  model.ReasoningTokens,
				CacheReadTokens:  model.CacheReadTokens,
				CacheWriteTokens: model.CacheWriteTokens,
			},
			Cost:  cost,
			Calls: model.Calls,
		}
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
		if _, err := runtimeidentity.ParseMember(rootID); err != nil {
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
		rootIDs, err := e.queryCheckpointRootIDs(ctx,
			`SELECT root_member_id FROM executor_checkpoints WHERE session_id = ? ORDER BY root_member_id`,
			sessionID,
		)
		if err != nil {
			return fmt.Errorf("sqlite: list executor checkpoints for Session %q: %w", sessionID, err)
		}
		for _, rootID := range rootIDs {
			if err := e.deleteOwnedCheckpoint(ctx, sessionID, rootID); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteUnownedCheckpoints removes checkpoint aggregates that are not in
// keepRootIDs.
// Boot reconciliation calls it after proving the exact set of waiting Run
// trees that still own resumable continuations.
func (e *ExecutorCheckpointStore) DeleteUnownedCheckpoints(ctx context.Context, keepRootIDs []string) error {
	keep := make(map[string]struct{}, len(keepRootIDs))
	for _, rootID := range keepRootIDs {
		if _, err := runtimeidentity.ParseMember(rootID); err != nil {
			return fmt.Errorf("sqlite: delete unowned executor checkpoints: %w", err)
		}
		if _, duplicate := keep[rootID]; duplicate {
			return fmt.Errorf("sqlite: delete unowned executor checkpoints: duplicate preserved root ID %q", rootID)
		}
		keep[rootID] = struct{}{}
	}
	return RunInTx(ctx, e.db, func(ctx context.Context) error {
		rootIDs, err := e.queryCheckpointRootIDs(ctx,
			`SELECT root_member_id FROM executor_checkpoints ORDER BY root_member_id`,
		)
		if err != nil {
			return fmt.Errorf("sqlite: list executor checkpoint roots: %w", err)
		}
		var stale []string
		for _, rootID := range rootIDs {
			if _, preserved := keep[rootID]; !preserved {
				stale = append(stale, rootID)
			}
		}
		for _, rootID := range stale {
			if err := e.deleteCheckpoint(ctx, rootID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (e *ExecutorCheckpointStore) queryCheckpointRootIDs(
	ctx context.Context,
	query string,
	args ...any,
) ([]string, error) {
	rows, err := conn(ctx, e.db).QueryContext(ctx, query, args...)
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

func (e *ExecutorCheckpointStore) deleteCheckpoint(ctx context.Context, rootMemberID string) error {
	if _, err := conn(ctx, e.db).ExecContext(ctx,
		`DELETE FROM executor_checkpoints WHERE root_member_id = ?`,
		rootMemberID,
	); err != nil {
		return fmt.Errorf("sqlite: delete executor checkpoint %q: %w", rootMemberID, err)
	}
	return nil
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
		run.ErrInvalidCheckpoint,
	)
}
