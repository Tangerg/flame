package plugins

import (
	"context"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

func (c *Coordinator) Component(ctx context.Context, id resourceid.InstallationID, digest fingerprint.Digest, component plugin.ContributionID) (plugin.ClientComponent, error) {
	var value plugin.ClientComponent
	err := c.dependencies.UnderAdmission(ctx, func(_ []plugin.Dependency) error {
		snapshot, err := c.installationSnapshot(ctx, id)
		if err != nil {
			return err
		}
		value, err = snapshot.Installation.AuthorizeComponent(snapshot.Selected, digest, component)
		return err
	})
	return value, err
}

func (c *Coordinator) Resource(ctx context.Context, id resourceid.InstallationID, digest fingerprint.Digest, component plugin.ContributionID, resource string) ([]byte, error) {
	if !plugin.ValidResourcePath(resource) || !strings.HasPrefix(resource, plugin.Namespace+"/") {
		return nil, plugin.ErrInvalid
	}
	var installation *plugin.Installation
	err := c.dependencies.UnderAdmission(ctx, func(_ []plugin.Dependency) error {
		snapshot, err := c.installationSnapshot(ctx, id)
		if err != nil {
			return err
		}
		if _, err := snapshot.Installation.AuthorizeComponent(snapshot.Selected, digest, component); err != nil {
			return err
		}
		installation = snapshot.Installation
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c.packages.ReadResource(ctx, installation, resource)
}
