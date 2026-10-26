package plugins

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/dependency"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
)

// RunActorAdmission reads installation authority inside the Run opening
// transaction. A concurrent disable commits either before acceptance or after it.
type RunActorAdmission struct{ store Store }

func NewRunActorAdmission(store Store) (*RunActorAdmission, error) {
	if dependency.Missing(store) {
		return nil, errors.New("plugins: actor installation store is required")
	}
	return &RunActorAdmission{store: store}, nil
}
func (a *RunActorAdmission) Admit(ctx context.Context, actor operation.Actor) error {
	snapshot, found, err := a.store.Get(ctx, actor.InstallationID())
	if err != nil {
		return err
	}
	if !found || !snapshot.Installation.Enabled() || snapshot.Selected.Digest() != actor.Release() {
		return fmt.Errorf("%w: installed Run actor was withdrawn", runs.ErrInstallationChanged)
	}
	if _, err := snapshot.Installation.ServerSource(snapshot.Selected, actor.Server()); err != nil {
		return err
	}
	if !snapshot.Installation.ServerEnabled(actor.Server()) || snapshot.Installation.ServerAuthority(actor.Server()) != actor.Authority() {
		return fmt.Errorf("%w: installed Run actor was withdrawn", runs.ErrInstallationChanged)
	}
	return nil
}
