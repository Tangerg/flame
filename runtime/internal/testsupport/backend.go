package testsupport

import (
	"context"
	json "encoding/json/v2"

	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/qimu/mcp"
	"github.com/Tangerg/qimu/portable"
)

func CallCapability[P, R any](ctx context.Context, session *sdk.ClientSession, contract *portable.Capability, export, method, id string, input P, metadata sdk.ParamsBase) (R, error) {
	var zero R
	exported, err := qimumcp.NewCapabilityBinding(session, export, contract)
	if err != nil {
		return zero, err
	}
	bound, err := exported.Method(method)
	if err != nil {
		return zero, err
	}
	admitted, err := jsonvalue.Encode(input)
	if err != nil {
		return zero, err
	}
	result, err := bound.InvokeWithMetadata(ctx, qimumcp.CommandID(id), admitted, metadata)
	if err != nil {
		return zero, err
	}
	err = json.Unmarshal(result.JSON(), &zero, json.RejectUnknownMembers(true))
	return zero, err
}

func ReceiptCapability[P, R any](ctx context.Context, session *sdk.ClientSession, contract *portable.Capability, export, method, id string, input P) (R, bool, error) {
	var zero R
	exported, err := qimumcp.NewCapabilityBinding(session, export, contract)
	if err != nil {
		return zero, false, err
	}
	bound, err := exported.Method(method)
	if err != nil {
		return zero, false, err
	}
	admitted, err := jsonvalue.Encode(input)
	if err != nil {
		return zero, false, err
	}
	result, found, err := bound.ReadReceipt(ctx, qimumcp.CommandID(id), admitted)
	if err != nil || !found {
		return zero, found, err
	}
	err = json.Unmarshal(result.JSON(), &zero, json.RejectUnknownMembers(true))
	return zero, true, err
}
