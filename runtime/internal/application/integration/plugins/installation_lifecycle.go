package plugins

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/telemetry"

	framehost "github.com/Tangerg/frame/host"
)

type installationCommitKey struct{}

type installationCommit func(context.Context, framehost.InstallationChange) error

func withInstallationCommit(ctx context.Context, commit installationCommit) context.Context {
	return context.WithValue(ctx, installationCommitKey{}, commit)
}

// The owner projection supplies the transaction for this admitted change.
func SettleInstallationCommit(ctx context.Context, change framehost.InstallationChange) error {
	if commit, found := ctx.Value(installationCommitKey{}).(installationCommit); found {
		return commit(ctx, change)
	}
	return nil
}

func (c *Coordinator) SetEnablement(ctx context.Context, id identity.InstallationID, enabled bool, reply operation.Reply[bool]) error {
	before, err := c.installationSnapshot(ctx, id)
	if err != nil {
		return err
	}
	if before.Installation.Enabled() == enabled {
		return reply(ctx, func(tx context.Context) (bool, error) {
			current, err := c.installationSnapshot(tx, id)
			if err != nil {
				return false, err
			}
			if current.Installation.Enabled() != enabled {
				return false, plugin.ErrStale
			}
			return enabled, nil
		})
	}

	installation := before.Installation
	if enabled {
		installation, err = installation.Enable(before.Selected)
		if err != nil {
			return err
		}
	} else {
		installation = installation.Disable()
	}
	bound := withInstallationCommit(ctx, func(tx context.Context, change framehost.InstallationChange) error {
		if change.After == nil || change.After.ID != id.Host() || change.After.Enabled != enabled {
			return plugin.ErrStale
		}
		return reply(tx, func(context.Context) (bool, error) {
			return change.After.Enabled, nil
		})
	})
	reconcile, err := c.commitInstallationChange(bound, before, installation, before.Selected)
	if framehost.InstallationConclusion(err) != framehost.Committed {
		return err
	}

	c.reconcileCommittedInstallation(ctx, id, reconcile, err)
	return nil
}

func (c *Coordinator) Uninstall(ctx context.Context, id identity.InstallationID, reply operation.Reply[struct{}]) error {
	var servers []mcpserver.ID
	bound := withInstallationCommit(ctx, func(tx context.Context, change framehost.InstallationChange) error {
		if change.Before == nil || change.After != nil || change.Before.ID != id.Host() {
			return plugin.ErrStale
		}
		return reply(tx, func(tx context.Context) (struct{}, error) {
			release, err := c.catalog.Get(tx, change.Before.Release)
			if err != nil {
				return struct{}{}, err
			}
			servers, err = release.InstallationServerIDs(id)
			return struct{}{}, err
		})
	})
	commitErr := c.framework.Uninstall(bound, id.Host())
	if framehost.InstallationConclusion(commitErr) != framehost.Committed {
		if errors.Is(commitErr, framehost.ErrUnavailable) {
			return errors.Join(plugin.ErrNotFound, commitErr)
		}
		return FrameworkError(commitErr)
	}

	ownerCtx, finish := c.realizationContext(ctx)
	defer finish()
	withdrawErr := c.connections.WithdrawInstallation(ownerCtx, servers)
	c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins}, invalidation.Notice{Resource: invalidation.Skills})
	reconcileErr := c.connections.ReconcileInstallation(ownerCtx, servers)
	if err := errors.Join(commitErr, withdrawErr, reconcileErr, c.Reclaim(ownerCtx)); err != nil {
		slog.WarnContext(ownerCtx, "committed plugin removal could not fully reconcile", telemetry.InstallationID(id), telemetry.Error(err))
	}
	return nil
}
