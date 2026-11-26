package plugins

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/telemetry"

	framehost "github.com/Tangerg/frame/host"
)

type enablementReceiptKey struct{}

type enablementReceipt struct {
	id      identity.InstallationID
	enabled bool
	reply   operation.Reply[plugin.Record]
}

func (c *Coordinator) SetEnablement(ctx context.Context, id identity.InstallationID, enabled bool, reply operation.Reply[plugin.Record]) error {
	before, err := c.installationSnapshot(ctx, id)
	if err != nil {
		return err
	}
	if before.Installation.Enabled() == enabled {
		return reply(ctx, func(tx context.Context) (plugin.Record, error) {
			current, err := c.installationSnapshot(tx, id)
			if err != nil {
				return plugin.Record{}, err
			}
			if current.Installation.Enabled() != enabled {
				return plugin.Record{}, plugin.ErrStale
			}
			return current.Installation.Snapshot(), nil
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
	bound := context.WithValue(ctx, enablementReceiptKey{}, enablementReceipt{id: id, enabled: enabled, reply: reply})
	reconcile, err := c.commitInstallationChange(bound, before, installation)
	if framehost.InstallationConclusion(err) != framehost.Committed {
		return err
	}

	ownerCtx, finish := c.realizationContext(ctx)
	defer finish()
	_, realizationErr := c.realizeInstallationChange(ownerCtx, id, reconcile)
	if err := errors.Join(err, realizationErr, c.Reclaim(ownerCtx)); err != nil {
		slog.WarnContext(ownerCtx, "committed plugin enablement could not fully reconcile", telemetry.InstallationID(id), telemetry.Error(err))
	}
	return nil
}

// This joins Frame's installation transaction through its owner projection.
func SettleInstallationEnablement(ctx context.Context, record plugin.Record) error {
	receipt, found := ctx.Value(enablementReceiptKey{}).(enablementReceipt)
	if !found {
		return nil
	}
	if record.ID != receipt.id || record.Enabled != receipt.enabled {
		return plugin.ErrStale
	}
	return receipt.reply(ctx, func(context.Context) (plugin.Record, error) {
		return record, nil
	})
}
