package client

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"reflect"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/scope/core/jsonschema"
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
func ResolvePointValues[T any](ctx context.Context, caller PointResolver, contract *portable.Point, target portable.Target, attributes any, order []protocol.PluginPointReference, options protocol.CallOptions) ([]PointValue[T], error) {
	if lo.IsNil(caller) || contract == nil {
		return nil, errors.New("runtime: point resolver and admitted contract are required")
	}
	if err := jsonvalue.ValidateType(reflect.TypeFor[T]()); err != nil {
		return nil, err
	}
	schema, err := jsonschema.For[T]()
	if err != nil {
		return nil, err
	}
	actual, err := jsonvalue.Parse(schema.JSON())
	if err != nil {
		return nil, err
	}
	required, err := jsonvalue.Parse(contract.Definition().Value)
	if err != nil {
		return nil, err
	}
	if actual != required {
		return nil, errors.New("runtime: consumer type differs from its point value schema")
	}
	contextJSON, err := jsonvalue.Encode(attributes)
	if err != nil {
		return nil, err
	}
	if order == nil {
		order = []protocol.PluginPointReference{}
	}
	page, err := caller.ResolvePluginPoint(ctx, protocol.ResolvePluginPointRequest{Contract: string(contract.Identity().JSON()), Target: target, Attributes: string(contextJSON.JSON()), Order: order}, options)
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
		input, err := jsonvalue.Parse([]byte(binding.Value))
		if err != nil {
			return nil, errors.Join(protocol.ErrInvalidResponse, err)
		}
		if err := contract.ValidateValue(input); err != nil {
			return nil, errors.Join(protocol.ErrInvalidResponse, err)
		}
		var value T
		if err := json.Unmarshal(input.JSON(), &value, json.RejectUnknownMembers(true), jsonvalue.Numbers()); err != nil {
			return nil, errors.Join(protocol.ErrInvalidResponse, err)
		}
		values = append(values, PointValue[T]{InstallationID: binding.InstallationID, Digest: binding.Digest, Contribution: binding.Contribution, Value: value})
	}
	return values, nil
}
