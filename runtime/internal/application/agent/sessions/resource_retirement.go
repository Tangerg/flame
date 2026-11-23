package sessions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

type ResourceRetirement struct {
	id        uuid.UUID
	sessionID identity.SessionID
}

func newResourceRetirement(sessionID identity.SessionID) (ResourceRetirement, error) {
	return RestoreResourceRetirement(uuid.NewString(), sessionID)
}

func RestoreResourceRetirement(id string, sessionID identity.SessionID) (ResourceRetirement, error) {
	if err := sessionID.Validate(); err != nil {
		return ResourceRetirement{}, fmt.Errorf("sessions: resource retirement: %w", err)
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return ResourceRetirement{}, fmt.Errorf("sessions: parse resource retirement identity: %w", err)
	}
	if parsed == uuid.Nil || parsed.String() != id {
		return ResourceRetirement{}, errors.New("sessions: resource retirement identity is not a canonical UUID")
	}
	return ResourceRetirement{id: parsed, sessionID: sessionID}, nil
}

func (r ResourceRetirement) ID() string                    { return r.id.String() }
func (r ResourceRetirement) SessionID() identity.SessionID { return r.sessionID }

func (r ResourceRetirement) Validate() error {
	_, err := RestoreResourceRetirement(r.ID(), r.SessionID())
	return err
}

type Cleanup struct {
	Resource string
	Retire   func(context.Context, identity.SessionID) error
}

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

func (r *ResourceRetirer) Settle(ctx context.Context, sessionID identity.SessionID) (bool, error) {
	retirement, found, err := r.retirements.Lookup(ctx, sessionID)
	if err != nil || !found {
		return err == nil, err
	}
	if err := r.complete(ctx, retirement); err != nil {
		slog.ErrorContext(ctx, "sessions: committed resource retirement is still pending",
			telemetry.SessionID(sessionID), telemetry.Error(err))
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
		return fmt.Errorf("%w: Session resource retirement changed", rundomain.ErrSessionBusy)
	}
	var errs []error
	for _, cleanup := range r.cleanups {
		if err := cleanup.Retire(ctx, retirement.SessionID()); err != nil {
			errs = append(errs, fmt.Errorf("sessions: retire %s: %w", cleanup.Resource, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	return r.retirements.Complete(ctx, retirement)
}

func (r *ResourceRetirer) RecoverPending(ctx context.Context, admissions recoveryClaims) error {
	pending, err := r.retirements.ListPending(ctx)
	if err != nil {
		return err
	}
	for _, retirement := range pending {
		release, ok, err := admissions.AcquireRecoverySession(ctx, retirement.SessionID())
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
				telemetry.SessionID(retirement.SessionID()), telemetry.Error(err))
		}
	}
	return nil
}

type recoveryClaims interface {
	AcquireRecoverySession(ctx context.Context, sessionID identity.SessionID) (release func(), ok bool, err error)
}
