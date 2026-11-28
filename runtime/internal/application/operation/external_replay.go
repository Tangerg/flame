package operation

import (
	"context"
	"errors"
)

func (r *Replay) InvokeExternal(
	ctx context.Context,
	key, fingerprint string,
	execute func(Reply[[]byte]) error,
	refuse func(error) ([]byte, error),
) ([]byte, error) {
	return r.InvokeRecoverable(ctx, key, fingerprint, func(retained bool) error {
		if retained {
			return ErrInProgress
		}
		return execute(func(ctx context.Context, write func(context.Context) ([]byte, error)) error {
			record, claimed, err := r.store.Claim(ctx, ReplayClaim{Key: key, Fingerprint: fingerprint})
			if err != nil {
				return err
			}
			if !claimed {
				return ErrInProgress
			}
			// A disconnected caller cannot distinguish a stopped process from a completed effect.
			work := context.WithoutCancel(ctx)
			record.Payload, err = write(work)
			if err != nil {
				return errors.Join(ErrOutcomePending, err)
			}
			_, err = r.persistOutcome(work, record)
			return err
		})
	}, refuse)
}
