package completion

import (
	"context"
	"errors"
)

func Wait(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("completion: wait context is required")
	}
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		select {
		case <-done:
			return nil
		default:
			return errors.Join(ctx.Err(), context.Cause(ctx))
		}
	}
}
