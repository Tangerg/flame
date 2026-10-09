package plugins

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

// AuthorizeAction admits a user command against the selected release. The
// product operation executes outside installation admission and remains owned
// by its existing use case after acceptance, including after withdrawal.
func (c *Coordinator) AuthorizeAction(ctx context.Context, id resourceid.InstallationID, digest fingerprint.Digest, actionID string, operation plugin.ActionOperation) error {
	snapshot, err := c.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if snapshot.Selected.Digest() != digest {
		return plugin.ErrStale
	}
	realization, err := c.packages.Realize(ctx, snapshot.Installation, snapshot.Selected)
	if err != nil {
		return err
	}
	return c.dependencies.UnderAdmission(ctx, func(_ []plugin.Dependency) error {
		current, err := c.store.Get(ctx, id)
		if err != nil {
			return err
		}
		if err := current.Installation.AuthorizeAction(current.Selected, digest, actionID, operation); err != nil {
			return err
		}
		if realization.Release != ReleaseAvailable {
			return plugin.ErrUnavailable
		}
		return nil
	})
}
