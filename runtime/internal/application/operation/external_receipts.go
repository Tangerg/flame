package operation

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
)

type ExternalExecution func(context.Context, uuid.UUID, []byte) error

type ExternalReply[T any] func(context.Context, T, ExternalExecution) (bool, error)

type ExternalCompletion func(context.Context, ExternalExecution, []byte) ([]byte, error)

type ExternalReceipts interface {
	// Absence permits the original invocation to finish preflight. An admitted
	// writer with no receipt returns ErrInProgress.
	ReadReceipt(uuid.UUID) ([]byte, error)
	RetireReceipt(uuid.UUID) error
}

func (r *Replay) InvokeOwnedExternal(
	ctx context.Context,
	key, fingerprint string,
	admit ReplayAdmission,
	receipts ExternalReceipts,
	prepare func(ExternalCompletion) error,
	refuse func(error) ([]byte, error),
) ([]byte, error) {
	release, err := r.inFlight.Acquire(ctx, key)
	if err != nil {
		return nil, err
	}
	defer release()
	record, found, err := r.store.Lookup(ctx, key, fingerprint)
	if err != nil {
		return nil, err
	}
	var id uuid.UUID
	if found {
		if record.Settled() {
			return settledPayload(record)
		}
		id, err = uuid.FromBytes(record.Settlement)
		if err != nil {
			return nil, ErrInProgress
		}
		if payload, err := r.receiveExternal(ctx, receipts, id, record); len(payload) != 0 || err != nil {
			return payload, err
		}
	}
	if err := admit.check(ctx); err != nil {
		return nil, err
	}

	var payload []byte
	invoked := false
	err = prepare(func(ctx context.Context, start ExternalExecution, success []byte) ([]byte, error) {
		invoked = true
		if id == uuid.Nil {
			reservedID, reserved, err := r.reserveExternal(ctx, key, fingerprint)
			if err != nil {
				return nil, err
			}
			id, record = reservedID, reserved
		}
		work := context.WithoutCancel(ctx)
		startErr := start(work, id, success)
		var readErr error
		payload, readErr = r.receiveExternal(work, receipts, id, record)
		if len(payload) != 0 {
			return payload, nil
		}
		if readErr == nil && startErr == nil {
			return nil, ErrClaimLost
		}
		return nil, errors.Join(readErr, startErr)
	})
	if invoked || ctx.Err() != nil {
		return payload, err
	}
	if err == nil {
		return nil, ErrClaimLost
	}
	if id == uuid.Nil {
		payload, _, err = r.commitOutcome(ctx, key, fingerprint, func(context.Context) ([]byte, error) {
			return refuse(err)
		})
		return payload, err
	}
	record.Payload, err = refuse(err)
	if err != nil {
		return nil, err
	}
	return r.finishRecord(ctx, record)
}

func (r *Replay) reserveExternal(ctx context.Context, key, fingerprint string) (uuid.UUID, ReplayRecord, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return uuid.Nil, ReplayRecord{}, err
	}
	var record ReplayRecord
	err = r.store.Transact(ctx, func(tx context.Context) error {
		reserved, claimed, err := r.store.Claim(tx, ReplayClaim{Key: key, Fingerprint: fingerprint})
		if err != nil {
			return err
		}
		if !claimed {
			return ErrInProgress
		}
		reserved.Settlement = id[:]
		record = reserved
		return r.store.StageSettlement(tx, record)
	})
	return id, record, err
}

func (r *Replay) receiveExternal(ctx context.Context, receipts ExternalReceipts, id uuid.UUID, record ReplayRecord) ([]byte, error) {
	payload, err := receipts.ReadReceipt(id)
	if len(payload) == 0 || err != nil {
		return nil, err
	}
	record.Payload = payload
	projected, err := r.finishRecord(ctx, record)
	if err != nil {
		slog.ErrorContext(ctx, "project external receipt into the replay journal", "error", err)
		return payload, nil
	}
	if err := receipts.RetireReceipt(id); err != nil {
		slog.ErrorContext(ctx, "retire projected external receipt", "error", err)
	}
	return projected, nil
}
