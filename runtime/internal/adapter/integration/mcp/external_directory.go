package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
	framecall "github.com/Tangerg/frame/call"
)

func (c *ExternalConnector) Save(ctx context.Context, server mcpserver.Server, receipt operation.ReplayClaim) (*mcpapp.DirectorySettlement, error) {
	return c.change(ctx, server, &server, receipt)
}

func (c *ExternalConnector) Remove(ctx context.Context, server mcpserver.Server, receipt operation.ReplayClaim) (*mcpapp.DirectorySettlement, error) {
	return c.change(ctx, server, nil, receipt)
}

func (c *ExternalConnector) change(ctx context.Context, server mcpserver.Server, replacement *mcpserver.Server, receipt operation.ReplayClaim) (settle *mcpapp.DirectorySettlement, err error) {
	u, err := c.acquire(ctx)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, plugin.ErrUnavailable
	}
	defer func() {
		if settle == nil {
			err = errors.Join(err, u.session.release(context.WithoutCancel(ctx)))
		}
	}()
	if server.DirectoryRevision != "" {
		source, err := mcpserver.ConnectorSource(u.source.InstallationID)
		if err != nil || source != server.Source {
			return nil, errors.Join(plugin.ErrStale, err)
		}
	}
	command := operation.Start(ctx, extension.Automatic, c.methods.Change.Name(), "")
	invocation, err := operation.Capture(command, framecall.Target{Scope: extension.ScopeInstallation, Reference: u.source.InstallationID.String()})
	if err != nil {
		return nil, err
	}
	input := wire.Change{Name: server.Name.String(), Expected: server.DirectoryRevision}
	if replacement != nil {
		timeout, _ := replacement.HandshakeTimeout.Duration()
		input.Replacement = &wire.Entry{
			Name: input.Name, Description: replacement.Description, Enabled: replacement.Enabled,
			HandshakeSeconds: int64(timeout / time.Second),
		}
	}
	pending, err := c.vault.PrepareDirectoryChange(ctx, mcpapp.PendingDirectoryChange{
		Intent: mcpapp.DirectoryIntent{Source: u.source.ContributionSource(), Invocation: invocation, Change: input}, Receipt: receipt,
	}, replacement)
	if err != nil {
		return nil, err
	}
	return c.settleDirectory(ctx, u, pending, false)
}

func (c *ExternalConnector) RecoverDirectoryChange(ctx context.Context, receipt operation.ReplayClaim) (*mcpapp.DirectorySettlement, error) {
	pending, found, err := c.vault.PendingDirectoryChange(ctx, receipt)
	if err != nil || !found {
		return nil, err
	}
	return c.resumeDirectoryChange(ctx, pending)
}

func (c *ExternalConnector) RestoreDirectoryChanges(ctx context.Context) error {
	pending, err := c.vault.PendingDirectoryChanges(ctx)
	if err != nil {
		return err
	}
	var result error
	for _, change := range pending {
		settlement, err := c.resumeDirectoryChange(ctx, change)
		if err == nil {
			err = settlement.Complete(ctx)
		}
		if err != nil {
			result = errors.Join(result, fmt.Errorf("mcp: settle directory change for %q: %w", change.Intent.Change.Name, err))
		}
	}
	return result
}

func (c *ExternalConnector) resumeDirectoryChange(ctx context.Context, pending mcpapp.PendingDirectoryChange) (settle *mcpapp.DirectorySettlement, err error) {
	u, err := c.acquireCaptured(ctx, pending.Intent.Source)
	if err != nil {
		return nil, errors.Join(operation.ErrOutcomePending, err)
	}
	defer func() {
		if settle == nil {
			err = errors.Join(err, u.session.release(context.WithoutCancel(ctx)))
		}
	}()
	return c.settleDirectory(ctx, u, pending, true)
}

func (c *ExternalConnector) settleDirectory(ctx context.Context, u *connectorUse, pending mcpapp.PendingDirectoryChange, recoverReceipt bool) (*mcpapp.DirectorySettlement, error) {
	intent := pending.Intent
	ctx, method, err := bindConnectorCommand(ctx, u, intent.Source, intent.Invocation, c.methods.Change)
	if err != nil {
		return nil, errors.Join(operation.ErrOutcomePending, err)
	}
	var decision wire.Decision
	var found bool
	id := pluginmcp.CommandID(intent.Invocation.ID)
	if recoverReceipt {
		decision, found, err = method.ReadReceipt(ctx, id, intent.Change)
	}
	if err == nil && !found {
		// The Connector commits the directory and its receipt in one transaction.
		// The exact retained command can reach that same owner again after a lost request.
		decision, err = method.CallCommand(ctx, id, intent.Change)
	}
	if err != nil {
		return nil, errors.Join(operation.ErrOutcomePending, err)
	}
	var refusal error
	switch decision {
	case wire.Applied:
	case wire.Conflict:
		refusal = plugin.ErrStale
		if intent.Change.Expected == "" {
			refusal = mcpapp.ErrServerAlreadyExists
		}
	default:
		return nil, errors.Join(operation.ErrOutcomePending, mcpapp.ErrInvalidServerConfiguration)
	}
	name, err := mcpserver.ParseServerName(intent.Change.Name)
	if err != nil {
		return nil, errors.Join(operation.ErrOutcomePending, err)
	}
	server, err := mcpserver.NewID(mcpserver.UserOrigin(), name)
	if err != nil {
		return nil, errors.Join(operation.ErrOutcomePending, err)
	}
	return &mcpapp.DirectorySettlement{
		Server: server, Refusal: refusal,
		Complete: func(ctx context.Context) error {
			defer func() {
				if err := u.session.release(context.WithoutCancel(ctx)); err != nil {
					slog.ErrorContext(ctx, "mcp: release directory connector", telemetry.MCPServer(server.String()), telemetry.Error(err))
				}
			}()
			var payload []byte
			if pending.Receipt.Key != "" {
				var err error
				payload, err = c.receipts.Directory(refusal)
				if err != nil {
					return errors.Join(operation.ErrOutcomePending, err)
				}
			}
			if err := c.vault.CompleteDirectoryChange(ctx, pending, refusal == nil, payload); err != nil {
				return errors.Join(operation.ErrOutcomePending, err)
			}
			return nil
		},
	}, nil
}

func bindConnectorCommand[P, R any](ctx context.Context, u *connectorUse, source extension.ContributionSource, invocation framecall.Invocation, operation *pluginmcp.Method[P, R]) (context.Context, *pluginmcp.BoundMethod[P, R], error) {
	if source != u.source.ContributionSource() {
		return ctx, nil, plugin.ErrStale
	}
	ctx, err := pluginmcp.WithInvocation(ctx, invocation)
	if err != nil {
		return ctx, nil, err
	}
	method, err := operation.Bind(u.capability)
	return ctx, method, err
}

func (c *ExternalConnector) verifyGrant(ctx context.Context, u *connectorUse, intent mcpapp.DirectoryIntent, token string) error {
	if intent.Change.Replacement == nil || intent.Change.Replacement.LaunchGrant != token {
		return mcpapp.ErrInvalidServerConfiguration
	}
	ctx, method, err := bindConnectorCommand(ctx, u, intent.Source, intent.Invocation, c.methods.Change)
	if err != nil {
		return err
	}
	decision, found, err := method.ReadReceipt(ctx, pluginmcp.CommandID(intent.Invocation.ID), intent.Change)
	if err != nil {
		return err
	}
	if !found {
		return plugin.ErrUnavailable
	}
	if decision != wire.Applied {
		return mcpapp.ErrInvalidServerConfiguration
	}
	return nil
}
