package sessions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/google/uuid"
	"github.com/samber/lo"
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

// Cleanup removes one family of resources a retired Session generation
// owned. Retire must be idempotent: a failed cleanup keeps the retirement
// pending and is attempted again.
type Cleanup struct {
	Resource string
	Retire   func(sessionID string) error
}

// ResourceRetirer alone completes committed resource retirements. Until every
// cleanup succeeds the durable record keeps the Session fenced; the next
// exclusive admission of that Session retries instead of waiting for a restart.
type ResourceRetirer struct {
	retirements resourceRetirements
	cleanups    []Cleanup
}

func NewResourceRetirer(retirements resourceRetirements, cleanups ...Cleanup) (*ResourceRetirer, error) {
	if lo.IsNil(retirements) {
		return nil, errors.New("sessions: resource retirement store is required")
	}
	for _, cleanup := range cleanups {
		if cleanup.Resource == "" || cleanup.Retire == nil {
			return nil, errors.New("sessions: resource cleanup requires a name and an action")
		}
	}
	return &ResourceRetirer{retirements: retirements, cleanups: slices.Clone(cleanups)}, nil
}

// Settle completes sessionID's committed retirement, if any, under the caller's
// exclusive admission. It reports false while a cleanup still fails; the cause
// is logged because the caller can only answer that the Session is busy.
func (r *ResourceRetirer) Settle(ctx context.Context, sessionID string) (bool, error) {
	retirement, found, err := r.retirements.Lookup(ctx, sessionID)
	if err != nil || !found {
		return err == nil, err
	}
	if err := r.complete(ctx, retirement); err != nil {
		slog.ErrorContext(ctx, "sessions: committed resource retirement is still pending",
			"session.id", sessionID, "error", err)
		return false, nil
	}
	return true, nil
}

func (r *ResourceRetirer) complete(ctx context.Context, retirement ResourceRetirement) error {
	committed, found, err := r.retirements.Lookup(ctx, retirement.SessionID())
	if err != nil || !found {
		return err
	}
	if committed != retirement {
		return fmt.Errorf("%w: Session resource retirement changed", ErrSessionBusy)
	}
	var errs []error
	for _, cleanup := range r.cleanups {
		if err := cleanup.Retire(retirement.SessionID()); err != nil {
			errs = append(errs, fmt.Errorf("sessions: retire %s: %w", cleanup.Resource, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	return r.retirements.Complete(ctx, retirement)
}

// RecoverPending settles every retirement left by an earlier process. One
// Session's failing cleanup keeps only that Session fenced; it never prevents
// the Runtime from serving the others.
func (r *ResourceRetirer) RecoverPending(ctx context.Context, admissions recoveryClaims) error {
	pending, err := r.retirements.ListPending(ctx)
	if err != nil {
		return err
	}
	for _, retirement := range pending {
		release, ok, err := admissions.AcquireRecoverySession(retirement.SessionID())
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		err = r.complete(ctx, retirement)
		release()
		if err != nil {
			slog.ErrorContext(ctx, "sessions: committed resource retirement is still pending",
				"session.id", retirement.SessionID(), "error", err)
		}
	}
	return nil
}

type recoveryClaims interface {
	AcquireRecoverySession(sessionID string) (release func(), ok bool, err error)
}
