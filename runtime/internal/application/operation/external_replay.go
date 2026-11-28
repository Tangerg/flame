package operation

import (
	"context"
	"errors"
)

func (r *Replay) InvokeExternal(
	ctx context.Context,
	key, fingerprint string,
	admit func(context.Context) error,
	execute func(Reply[[]byte]) error,
	refuse func(error) ([]byte, error),
) ([]byte, error) {
	release, err := r.inFlight.Acquire(ctx, key)
	if err != nil {
		return nil, err
	}
	defer release()
	previous, found, err := r.store.Lookup(ctx, key, fingerprint)
	if err != nil {
		return nil, err
	}
	if found {
		return settledPayload(previous)
	}
	if err := admit(ctx); err != nil {
		return nil, err
	}

	var payload []byte
	attempted := false
	err = execute(func(ctx context.Context, write func(context.Context) ([]byte, error)) error {
		attempted = true
		record, claimed, err := r.store.Claim(ctx, ReplayClaim{Key: key, Fingerprint: fingerprint})
		if err != nil {
			return err
		}
		if !claimed {
			payload, err = settledPayload(record)
			return err
		}
		// A disconnected caller cannot distinguish a stopped process from a completed effect.
		work := context.WithoutCancel(ctx)
		record.Payload, err = write(work)
		if err != nil {
			return errors.Join(ErrOutcomePending, err)
		}
		payload, err = r.finishRecord(work, record)
		return err
	})
	switch {
	case err == nil && len(payload) != 0:
		return payload, nil
	case err == nil:
		return nil, ErrClaimLost
	case attempted || ctx.Err() != nil:
		return nil, err
	}
	payload, _, err = r.commitOutcome(ctx, key, fingerprint, func(context.Context) ([]byte, error) {
		return refuse(err)
	})
	return payload, err
}
