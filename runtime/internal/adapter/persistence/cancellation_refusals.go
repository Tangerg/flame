package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

type cancellationRefusal string

const (
	cancellationTargetFinished cancellationRefusal = "target_finished"
	cancellationTargetMissing  cancellationRefusal = "target_missing"
	cancellationUnconfirmed    cancellationRefusal = "unconfirmed"
)

func (r cancellationRefusal) cause() error {
	switch r {
	case cancellationTargetFinished:
		return runs.ErrRunFinished
	case cancellationTargetMissing:
		return runs.ErrRunNotFound
	default:
		return &runs.CancellationSettlementError{}
	}
}

func (i *idempotencyStore) refuseLateCancellation(ctx context.Context, runID identity.RunID) (cancellationRefusal, error) {
	if runID.IsZero() {
		return "", nil
	}
	var raw string
	err := i.db.connection(ctx).QueryRowContext(ctx, `SELECT state FROM runs WHERE run_id = ?`, runID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return cancellationTargetMissing, nil
	}
	if err != nil {
		return "", fmt.Errorf("sqlite: read cancellation target %q: %w", runID, err)
	}
	state, err := parseRunState(raw)
	if err != nil {
		return "", err
	}
	if state.IsTerminal() {
		return cancellationTargetFinished, nil
	}
	return "", nil
}

func (i *idempotencyStore) settleRunCancellationRefusals(ctx context.Context, value rundomain.Run, winningCancellationKey string) error {
	denial := runs.TerminalCancellationRefusal(value)
	if denial == nil {
		return nil
	}
	refusal := cancellationTargetFinished
	if _, unconfirmed := errors.AsType[*runs.CancellationSettlementError](denial); unconfirmed {
		refusal = cancellationUnconfirmed
	}
	_, err := i.db.connection(ctx).ExecContext(ctx, `UPDATE idempotency_records
		SET kind = ?, payload = ?, expires_at = ?, material_run_id = NULL
		WHERE kind = ? AND material_run_id = ? AND length(payload) = 0 AND key <> ?`,
		idempotencyCancellationRefusal, []byte(refusal), time.Now().Add(operation.ReplayRetention).Unix(),
		idempotencyPendingResponse, value.ID(), winningCancellationKey)
	if err != nil {
		return fmt.Errorf("sqlite: settle cancellation refusals for Run %q: %w", value.ID(), err)
	}
	return nil
}
