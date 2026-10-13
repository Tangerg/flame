package plugins

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

func (c *Coordinator) ReadRenderer(ctx context.Context, id resourceid.InstallationID, digest fingerprint.Digest, rendererID string) (string, error) {
	var installation *plugin.Installation
	var renderer plugin.Renderer
	err := c.dependencies.UnderAdmission(ctx, func(_ []plugin.Dependency) error {
		snapshot, err := c.installationSnapshot(ctx, id)
		if err != nil {
			return err
		}
		renderer, err = snapshot.Installation.AuthorizeRenderer(snapshot.Selected, digest, rendererID)
		installation = snapshot.Installation
		return err
	})
	if err != nil {
		return "", err
	}
	return c.packages.ReadPage(ctx, installation, renderer.Entry)
}
