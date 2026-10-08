package plugins

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

func (c *Coordinator) ReadView(ctx context.Context, id resourceid.InstallationID, digest fingerprint.Digest, viewID string) (html string, err error) {
	err = c.WithView(ctx, id, digest, viewID, func(installation *plugin.Installation, view plugin.ViewDeclaration) error {
		var err error
		html, err = c.packages.ReadView(ctx, installation, view)
		return err
	})
	return html, err
}

// Authorization shares installation admission; the accepted read runs outside
// that lock. Withdrawal refuses subsequent reads without waiting for storage I/O.
func (c *Coordinator) WithView(ctx context.Context, id resourceid.InstallationID, digest fingerprint.Digest, viewID string, read func(*plugin.Installation, plugin.ViewDeclaration) error) error {
	var installation *plugin.Installation
	var view plugin.ViewDeclaration
	err := c.dependencies.UnderAdmission(ctx, func(_ []plugin.Dependency) error {
		snapshot, err := c.store.Get(ctx, id)
		if err != nil {
			return err
		}
		view, err = snapshot.Installation.AuthorizeView(snapshot.Selected, digest, viewID)
		if err != nil {
			return err
		}
		installation = snapshot.Installation
		return nil
	})
	if err != nil {
		return err
	}
	return read(installation, view)
}
