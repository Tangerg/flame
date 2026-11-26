package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

var errAuthorizationReceiptUnavailable = errors.New("mcp: original authorization start receipt is unavailable")

func (c *ExternalConnector) RecoverAuthorizationStart(ctx context.Context, receipt operation.ReplayClaim) (*mcpapp.AuthorizationStartSettlement, error) {
	pending, found, err := c.vault.PendingAuthorizationStart(ctx, receipt)
	if err != nil || !found {
		return nil, err
	}
	return c.resumeAuthorizationStart(ctx, pending)
}

func (c *ExternalConnector) restoreAuthorizationStarts(ctx context.Context) error {
	starts, err := c.vault.PendingAuthorizationStarts(ctx)
	if err != nil {
		return err
	}
	epoch := *c.epoch.Load()
	var result error
	for _, pending := range starts {
		if pending.Intent.Start.Epoch == epoch {
			continue
		}
		settlement, err := c.resumeAuthorizationStart(ctx, pending)
		if err == nil {
			err = settlement.Complete(ctx)
		}
		if err != nil {
			result = errors.Join(result, fmt.Errorf("mcp: restore authorization attempt %q: %w", pending.Intent.Start.AttemptID, err))
		}
	}
	return result
}

func (c *ExternalConnector) resumeAuthorizationStart(ctx context.Context, pending mcpapp.PendingAuthorizationStart) (settlement *mcpapp.AuthorizationStartSettlement, returned error) {
	u, err := c.acquireCaptured(ctx, pending.Intent.Source)
	if err != nil {
		return nil, errors.Join(operation.ErrOutcomePending, err)
	}
	defer func() {
		if settlement == nil {
			returned = errors.Join(returned, u.session.release(context.WithoutCancel(ctx)))
		}
	}()
	return c.settleAuthorizationStart(ctx, u, pending, true)
}

func (c *ExternalConnector) settleAuthorizationStart(ctx context.Context, u *connectorUse, pending mcpapp.PendingAuthorizationStart, recoverReceipt bool) (*mcpapp.AuthorizationStartSettlement, error) {
	intent := pending.Intent
	ctx, method, err := bindConnectorCommand(ctx, u, intent.Source, intent.Invocation, c.methods.Start)
	if err != nil {
		return nil, errors.Join(operation.ErrOutcomePending, err)
	}
	result, err := readAuthorizationStart(ctx, method, intent, u.epoch, recoverReceipt)
	if err != nil {
		return nil, errors.Join(operation.ErrOutcomePending, err)
	}
	settlement, err := c.authorizationStartSettlement(ctx, intent, result, u.epoch, recoverReceipt)
	if err != nil {
		return nil, errors.Join(operation.ErrOutcomePending, err)
	}
	settlement.Complete = func(ctx context.Context) error {
		defer func() {
			if err := u.session.release(context.WithoutCancel(ctx)); err != nil {
				slog.ErrorContext(ctx, "mcp: release authorization connector", telemetry.MCPServer(settlement.Server.String()), telemetry.Error(err))
			}
		}()
		var payload []byte
		if pending.Receipt.Key != "" {
			var err error
			payload, err = c.receipts.Authorization(settlement.AttemptID, settlement.Refusal)
			if err != nil {
				return errors.Join(operation.ErrOutcomePending, err)
			}
		}
		if err := c.vault.CompleteAuthorizationStart(ctx, pending, settlement.Refusal == nil, payload); err != nil {
			return errors.Join(operation.ErrOutcomePending, err)
		}
		return nil
	}
	return settlement, nil
}

func readAuthorizationStart(ctx context.Context, method *pluginmcp.BoundMethod[wire.Start, wire.AttemptResult], intent mcpapp.AuthorizationIntent, epoch string, recoverReceipt bool) (wire.AttemptResult, error) {
	id := pluginmcp.CommandID(intent.Invocation.ID)
	if recoverReceipt {
		result, found, err := method.ReadReceipt(ctx, id, intent.Start)
		if err != nil || found {
			return result, err
		}
	}
	if intent.Start.Epoch != epoch {
		return wire.AttemptResult{}, errAuthorizationReceiptUnavailable
	}
	return method.CallCommand(ctx, id, intent.Start)
}

func (c *ExternalConnector) authorizationStartSettlement(ctx context.Context, intent mcpapp.AuthorizationIntent, result wire.AttemptResult, epoch string, checkPending bool) (*mcpapp.AuthorizationStartSettlement, error) {
	name, err := mcpserver.ParseServerName(intent.Start.Name)
	if err != nil {
		return nil, err
	}
	server, err := mcpserver.NewID(mcpserver.UserOrigin(), name)
	if err != nil {
		return nil, err
	}
	settlement := &mcpapp.AuthorizationStartSettlement{Server: server}
	if result.Attempt == nil {
		settlement.Refusal = mcpapp.ErrUnknownServer
		return settlement, nil
	}
	id, err := mcpapp.ParseAuthorizationAttemptID(intent.Start.AttemptID)
	if err != nil {
		return nil, err
	}
	accepted, err := readExternalAttempt(result, id)
	if err != nil {
		return nil, err
	}
	if accepted.Server != server || accepted.Status != mcpapp.AuthorizationAttemptPending {
		return nil, mcpapp.ErrInvalidServerConfiguration
	}
	settlement.AttemptID = id
	if intent.Start.Epoch != epoch {
		return settlement, nil
	}
	start := operation.Adopt(ctx, intent.Invocation.ID, intent.Invocation.Origin, intent.Invocation.Parent)
	finishContext := operation.Start(start, extension.Automatic, c.methods.Finish.Name(), id.String())
	invocation, err := operation.Capture(finishContext, intent.Invocation.Target)
	if err != nil {
		return nil, err
	}
	settlement.Transport = &mcpapp.AuthorizationTransport{
		Recipient:    intent.Recipient,
		CheckPending: checkPending,
		Report: func(ctx context.Context, status mcpapp.AuthorizationAttemptStatus) error {
			return c.recordAuthorizationFinish(ctx, mcpapp.AuthorizationFinish{ID: id, Invocation: invocation, Outcome: wire.Outcome(status)})
		},
	}
	return settlement, nil
}
