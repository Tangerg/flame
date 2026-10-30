package plugins

import (
	"context"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func (c *Coordinator) Component(ctx context.Context, id identity.InstallationID, digest identity.Digest, component plugin.ContributionID) (plugin.ClientComponent, error) {
	var value plugin.ClientComponent
	err := c.framework.Admission().Admit(ctx, func(ctx context.Context) error {
		snapshot, err := c.installationSnapshot(ctx, id)
		if err != nil {
			return err
		}
		value, err = snapshot.Installation.AuthorizeComponent(snapshot.Selected, digest, component)
		return err
	})
	return value, err
}

func (c *Coordinator) Resource(ctx context.Context, id identity.InstallationID, digest identity.Digest, component plugin.ContributionID, resource string) ([]byte, error) {
	if !plugin.ValidResourcePath(resource) || !strings.HasPrefix(resource, plugin.Namespace+"/") {
		return nil, plugin.ErrInvalid
	}
	var installation *plugin.Installation
	err := c.framework.Admission().Admit(ctx, func(ctx context.Context) error {
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

func (c *Coordinator) Locale(ctx context.Context, id identity.InstallationID, digest identity.Digest, tag string) (map[string]string, error) {
	var installation *plugin.Installation
	var locale plugin.Locale
	err := c.framework.Admission().Admit(ctx, func(ctx context.Context) error {
		snapshot, err := c.installationSnapshot(ctx, id)
		if err != nil {
			return err
		}
		locale, err = snapshot.Installation.AuthorizeLocale(snapshot.Selected, digest, tag)
		if err != nil {
			return err
		}
		installation = snapshot.Installation
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c.packages.ReadLocale(ctx, installation, locale)
}
