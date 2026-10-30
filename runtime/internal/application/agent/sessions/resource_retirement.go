package sessions

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/google/uuid"
)

// ResourceRetirement names the resource generation retired by one committed
// Session write-set. Only that transaction may persist its cleanup authority;
// constructing this value never authorizes filesystem deletion.
type ResourceRetirement struct {
	id        uuid.UUID
	sessionID identity.SessionID
}

func newResourceRetirement(sessionID string) (ResourceRetirement, error) {
	return RestoreResourceRetirement(uuid.NewString(), sessionID)
}

func RestoreResourceRetirement(id, sessionID string) (ResourceRetirement, error) {
	owner, err := identity.ParseSession(sessionID)
	if err != nil {
		return ResourceRetirement{}, fmt.Errorf("sessions: resource retirement: %w", err)
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return ResourceRetirement{}, fmt.Errorf("sessions: parse resource retirement identity: %w", err)
	}
	if parsed == uuid.Nil || parsed.String() != id {
		return ResourceRetirement{}, errors.New("sessions: resource retirement identity is not a canonical UUID")
	}
	return ResourceRetirement{id: parsed, sessionID: owner}, nil
}

func (r ResourceRetirement) ID() string        { return r.id.String() }
func (r ResourceRetirement) SessionID() string { return r.sessionID.String() }

func (r ResourceRetirement) Validate() error {
	_, err := RestoreResourceRetirement(r.ID(), r.SessionID())
	return err
}

func (c *Coordinator) retireCommittedResources(ctx context.Context, retirement ResourceRetirement) error {
	committed, found, err := c.retirements.Lookup(ctx, retirement.SessionID())
	if err != nil || !found {
		return err
	}
	if committed != retirement {
		return fmt.Errorf("%w: Session resource retirement changed", ErrSessionBusy)
	}
	var errs []error
	if c.checkpoints != nil {
		if err := c.checkpoints.DropSession(retirement.SessionID()); err != nil {
			errs = append(errs, fmt.Errorf("sessions: retire checkpoints: %w", err))
		}
	}
	if c.sandbox != nil {
		if err := c.sandbox.Discard(retirement.SessionID()); err != nil {
			errs = append(errs, fmt.Errorf("sessions: retire sandbox: %w", err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	return c.retirements.Complete(ctx, retirement)
}

// RecoverRetiredResources consumes only decisions committed with a history
// mutation. Its recovery claim wakes foreground waiters after durable completion.
func (c *Coordinator) RecoverRetiredResources(ctx context.Context) error {
	pending, err := c.retirements.ListPending(ctx)
	if err != nil {
		return err
	}
	for _, retirement := range pending {
		release, ok, err := c.admissions.AcquireRecoverySession(retirement.SessionID())
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: retiring Session resources", ErrSessionBusy)
		}
		err = c.retireCommittedResources(ctx, retirement)
		release()
		if err != nil {
			return fmt.Errorf("sessions: recover resources for %q: %w", retirement.SessionID(), err)
		}
	}
	return nil
}
