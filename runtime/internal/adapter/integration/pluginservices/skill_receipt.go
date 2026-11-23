package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func originalSkillReceipt[R any](ctx context.Context, calls *CommandCalls, record plugin.CommandBinding) (_ R, returned error) {
	var value R
	source, err := calls.original(ctx, record)
	if err != nil {
		return value, err
	}
	ctx, err = pluginmcp.WithInvocation(ctx, record.Invocation)
	if err != nil {
		return value, err
	}
	connection, err := calls.pool.BorrowActivation(ctx, source.ServerID, record.Activation)
	if err != nil {
		return value, err
	}
	defer func() { returned = errors.Join(returned, connection.Close()) }()
	contract, err := plugincontract.ParseCapability(record.Contract.Bytes())
	if err != nil {
		return value, err
	}
	export, err := pluginmcp.NewCapabilityBinding(connection.Session(), record.Export, contract)
	if err != nil {
		return value, err
	}
	method, err := export.Method(record.Method)
	if err != nil {
		return value, err
	}
	reply, found, err := method.ReadReceipt(connection.Context(), pluginmcp.CommandID(record.ID), record.Input)
	if err != nil {
		return value, err
	}
	if !found {
		return value, plugin.ErrCommandUnconfirmed
	}
	err = json.Unmarshal(reply.Bytes(), &value, json.RejectUnknownMembers(true))
	return value, err
}
