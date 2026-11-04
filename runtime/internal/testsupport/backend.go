package testsupport

import (
	"context"
	json "encoding/json/v2"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	framevalue "github.com/Tangerg/frame/value"

	sdk "github.com/Tangerg/go-sdk/mcp"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
)

func CallCapability[P, R any](ctx context.Context, session *sdk.ClientSession, contract *plugincontract.Capability, export, method, id string, input P, metadata sdk.ParamsBase) (R, error) {
	var zero R
	exported, err := pluginmcp.NewCapabilityBinding(session, export, contract)
	if err != nil {
		return zero, err
	}
	bound, err := exported.Method(method)
	if err != nil {
		return zero, err
	}
	admitted, err := framevalue.EncodeJSON(input)
	if err != nil {
		return zero, err
	}
	result, err := bound.InvokeWithMetadata(ctx, pluginmcp.CommandID(id), admitted, metadata)
	if err != nil {
		return zero, err
	}
	err = json.Unmarshal(result.Bytes(), &zero, json.RejectUnknownMembers(true))
	return zero, err
}

func ReceiptCapability[P, R any](ctx context.Context, session *sdk.ClientSession, contract *plugincontract.Capability, export, method, id string, input P) (R, bool, error) {
	var zero R
	exported, err := pluginmcp.NewCapabilityBinding(session, export, contract)
	if err != nil {
		return zero, false, err
	}
	bound, err := exported.Method(method)
	if err != nil {
		return zero, false, err
	}
	admitted, err := framevalue.EncodeJSON(input)
	if err != nil {
		return zero, false, err
	}
	result, found, err := bound.ReadReceipt(ctx, pluginmcp.CommandID(id), admitted)
	if err != nil || !found {
		return zero, found, err
	}
	err = json.Unmarshal(result.Bytes(), &zero, json.RejectUnknownMembers(true))
	return zero, true, err
}
