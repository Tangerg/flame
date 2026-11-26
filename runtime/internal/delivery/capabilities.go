package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func (e *Endpoint) enforceCapabilities(ctx context.Context, meta MethodMeta, members protocol.WireMembers) *protocol.Failure {
	for _, rule := range meta.CapabilityRules {
		if !matchesAll(rule.When, members) {
			continue
		}
		missing, err := e.missingFeatureRequirements(ctx, rule.Requires)
		if err != nil {
			return projectError(err)
		}
		if len(missing) != 0 {
			return projectError(newCapabilityGapError(missing...))
		}
	}
	return nil
}

func (e *Endpoint) missingFeatureRequirements(
	ctx context.Context,
	required []string,
) ([]protocol.CapabilityRequirement, error) {
	discoverer, ok := e.target.(interface {
		Discover(context.Context) (*protocol.DiscoverResponse, error)
	})
	if !ok || !capabilityAvailable(discoverer) {
		return nil, errors.New("operation: target cannot handle runtime.discover")
	}
	discovered, err := discoverer.Discover(ctx)
	if err != nil {
		return nil, fmt.Errorf("operation: read capabilities: %w", err)
	}
	if discovered == nil {
		return nil, errors.New("operation: the runtime reported no capabilities")
	}
	client, _ := clientCapabilitiesFrom(ctx)
	return protocol.MissingFeatureRequirements(
		discovered.Capabilities.Features,
		client,
		required...,
	), nil
}

func matchesAll(conditions []FieldCondition, members protocol.WireMembers) bool {
	for _, condition := range conditions {
		if !condition.matches(members) {
			return false
		}
	}
	return true
}

func (f FieldCondition) matches(members protocol.WireMembers) bool {
	switch f.Operator {
	case OperatorPresent:
		return members.Has(f.Field)
	case OperatorEquals:
		return members.StringEquals(f.Field, f.Value)
	case OperatorTrue:
		return members.IsTrue(f.Field)
	default:
		return false
	}
}
