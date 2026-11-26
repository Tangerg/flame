package plugins

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framevalue "github.com/Tangerg/frame/value"
)

func (c *Coordinator) Configure(ctx context.Context, id identity.InstallationID, digest framevalue.Digest, configuration plugin.Configuration, reply operation.Reply[framevalue.Digest]) error {
	before, installation, err := c.planConfiguration(ctx, id, digest, configuration)
	if err != nil {
		return err
	}
	if before.Installation.SameConfiguration(installation) {
		return c.transact(ctx, func(tx context.Context) error {
			current, next, err := c.planConfiguration(tx, id, digest, configuration)
			if err != nil {
				return err
			}
			if !current.Installation.SameConfiguration(next) {
				return plugin.ErrStale
			}
			return reply(tx, func(context.Context) (framevalue.Digest, error) {
				return current.Selected.Digest(), nil
			})
		})
	}
	return c.commitWithReleaseReceipt(ctx, before, installation, before.Selected, reply)
}

func (c *Coordinator) planConfiguration(ctx context.Context, id identity.InstallationID, digest framevalue.Digest, configuration plugin.Configuration) (plugin.Snapshot, plugin.Installation, error) {
	current, err := c.installationSnapshot(ctx, id)
	if err != nil {
		return plugin.Snapshot{}, plugin.Installation{}, err
	}
	if current.Selected.Digest() != digest {
		return plugin.Snapshot{}, plugin.Installation{}, fmt.Errorf("%w: release %s is not selected", plugin.ErrStale, digest)
	}
	installation, err := current.Installation.Configure(current.Selected, configuration)
	return current, installation, err
}
