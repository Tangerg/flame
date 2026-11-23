package client

import (
	"context"
	"errors"
	"reflect"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
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

func ResolvePointValues[T any](ctx context.Context, caller PointResolver, contract *plugincontract.Point, target framecall.Target, attributes any, order []protocol.PluginPointReference, options protocol.CallOptions) ([]PointValue[T], error) {
	if lo.IsNil(caller) || contract == nil {
		return nil, errors.New("runtime: point resolver and admitted contract are required")
	}
	contract = new(*contract)
	if err := requirePointValueType[T](contract); err != nil {
		return nil, err
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
		value, err := decodePointValue[T](contract, binding.Value)
		if err != nil {
			return nil, errors.Join(protocol.ErrInvalidResponse, err)
		}
		values = append(values, PointValue[T]{InstallationID: binding.InstallationID, Digest: binding.Digest, Contribution: binding.Contribution, Value: value})
	}
	return values, nil
}

func requirePointValueType[T any](contract *plugincontract.Point) error {
	if err := framevalue.ValidateType(reflect.TypeFor[T]()); err != nil {
		return err
	}
	schema, err := framevalue.SchemaFor[T]()
	if err != nil {
		return err
	}
	actual, err := framevalue.ParseJSON(schema)
	if err != nil {
		return err
	}
	required, err := framevalue.ParseJSON(contract.Definition().Value)
	if err != nil {
		return err
	}
	if actual != required {
		return errors.New("runtime: consumer type differs from its point value schema")
	}
	return nil
}

func decodePointValue[T any](contract *plugincontract.Point, encoded string) (T, error) {
	var value T
	input, err := framevalue.ParseJSON([]byte(encoded))
	if err != nil {
		return value, err
	}
	if err := contract.ValidateValue(input); err != nil {
		return value, err
	}
	err = input.Decode(&value)
	return value, err
}
