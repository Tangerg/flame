package client

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"reflect"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/samber/lo"
)

type PointResolver interface {
	ResolvePluginPoint(context.Context, protocol.ResolvePluginPointRequest, protocol.CallOptions) (*protocol.Page[protocol.PluginPointBinding], error)
}

type PointValue[T any] struct {
	InstallationID string
	Digest         string
	Contribution   string
	Value          T
}

// ResolvePointValues checks the consumer's typed schema against its published
// point before decoding immutable installed bindings into consumer-owned values.
func ResolvePointValues[T any](ctx context.Context, caller PointResolver, contract *plugincontract.Point, target portable.Target, attributes any, order []protocol.PluginPointReference, options protocol.CallOptions) ([]PointValue[T], error) {
	if lo.IsNil(caller) || contract == nil {
		return nil, errors.New("runtime: point resolver and admitted contract are required")
	}
	contract = new(*contract)
	if err := framevalue.ValidateType(reflect.TypeFor[T]()); err != nil {
		return nil, err
	}
	schema, err := portable.SchemaFor[T]()
	if err != nil {
		return nil, err
	}
	actual, err := framevalue.ParseJSON(schema)
	if err != nil {
		return nil, err
	}
	required, err := framevalue.ParseJSON(contract.Definition().Value)
	if err != nil {
		return nil, err
	}
	if actual != required {
		return nil, errors.New("runtime: consumer type differs from its point value schema")
	}
	contextJSON, err := framevalue.EncodeJSON(attributes)
	if err != nil {
		return nil, err
	}
	if order == nil {
		order = []protocol.PluginPointReference{}
	}
	page, err := caller.ResolvePluginPoint(ctx, protocol.ResolvePluginPointRequest{Contract: string(contract.Identity().Bytes()), Target: target, Attributes: string(contextJSON.Bytes()), Order: order}, options)
	if err != nil {
		return nil, err
	}
	if page == nil || page.NextCursor != "" {
		return nil, protocol.ErrInvalidResponse
	}
	if err := protocol.ValidateWireTree(*page); err != nil {
		return nil, errors.Join(protocol.ErrInvalidResponse, err)
	}
	values := make([]PointValue[T], 0, len(page.Data))
	for _, binding := range page.Data {
		input, err := framevalue.ParseJSON([]byte(binding.Value))
		if err != nil {
			return nil, errors.Join(protocol.ErrInvalidResponse, err)
		}
		if err := contract.ValidateValue(input); err != nil {
			return nil, errors.Join(protocol.ErrInvalidResponse, err)
		}
		var value T
		if err := json.Unmarshal(input.Bytes(), &value, json.RejectUnknownMembers(true), framevalue.Numbers()); err != nil {
			return nil, errors.Join(protocol.ErrInvalidResponse, err)
		}
		values = append(values, PointValue[T]{InstallationID: binding.InstallationID, Digest: binding.Digest, Contribution: binding.Contribution, Value: value})
	}
	return values, nil
}
