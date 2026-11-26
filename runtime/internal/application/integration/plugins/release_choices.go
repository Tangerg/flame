package plugins

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/telemetry"

	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
)

func (c *Coordinator) Stage(ctx context.Context, id identity.InstallationID, source string, reply operation.Reply[plugin.StagedRelease]) (err error) {
	candidate, err := c.packages.Materialize(ctx, source)
	if err != nil {
		return fmt.Errorf("plugins: materialize package: %w", err)
	}
	defer closeCandidate(ctx, candidate, id, &err)
	published, err := candidate.Publish(ctx)
	if err != nil {
		return err
	}
	err = c.transact(ctx, func(tx context.Context) error {
		snapshot, err := c.installationSnapshot(tx, id)
		if err != nil {
			return err
		}
		staged, err := snapshot.Installation.Stage(snapshot.Selected, published)
		if err != nil {
			return err
		}
		return reply(tx, func(tx context.Context) (plugin.StagedRelease, error) {
			return staged, c.store.Stage(tx, staged)
		})
	})
	if err != nil {
		return err
	}
	c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins})
	ownerCtx, finish := c.realizationContext(ctx)
	defer finish()
	if err := c.Reclaim(ownerCtx); err != nil {
		slog.WarnContext(ownerCtx, "committed plugin staging could not reclaim releases", telemetry.InstallationID(id), telemetry.Error(err))
	}
	return nil
}

func (c *Coordinator) Select(ctx context.Context, id identity.InstallationID, digest framevalue.Digest, reply operation.Reply[framevalue.Digest]) error {
	before, err := c.installationSnapshot(ctx, id)
	if err != nil {
		return err
	}
	if before.Staged == nil || before.Staged.Digest() != digest {
		return fmt.Errorf("%w: release %s is not staged", plugin.ErrStale, digest)
	}
	selected := *before.Staged
	installation, err := before.Installation.Select(before.Selected, selected)
	if err != nil {
		return err
	}
	return c.commitWithReleaseReceipt(ctx, before, installation, selected, reply)
}

func (c *Coordinator) commitWithReleaseReceipt(ctx context.Context, before plugin.Snapshot, installation plugin.Installation, selected plugin.Release, reply operation.Reply[framevalue.Digest]) error {
	id, digest := installation.ID(), selected.Digest()
	bound := withInstallationCommit(ctx, func(tx context.Context, change framehost.InstallationChange) error {
		if change.After == nil || change.After.ID != id.Host() || change.After.Release != digest {
			return plugin.ErrStale
		}
		return reply(tx, func(context.Context) (framevalue.Digest, error) {
			return change.After.Release, nil
		})
	})
	reconcile, err := c.commitInstallationChange(bound, before, installation, selected)
	if framehost.InstallationConclusion(err) != framehost.Committed {
		return err
	}
	c.reconcileCommittedInstallation(ctx, id, reconcile, err)
	return nil
}

func closeCandidate(ctx context.Context, candidate Candidate, id identity.InstallationID, outcome *error) {
	if err := candidate.Discard(); err != nil {
		if *outcome != nil {
			*outcome = errors.Join(*outcome, err)
			return
		}
		slog.WarnContext(ctx, "committed plugin command could not release its candidate", telemetry.InstallationID(id), telemetry.Error(err))
	}
}
