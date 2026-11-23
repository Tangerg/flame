package delivery

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// A declared capability this build cannot honor is a refusal, never a silent drop:
// the client would fold a stream expecting events that never come.
func (s *Handler) negotiateCapabilities(ctx context.Context) (run.Capabilities, error) {
	caps, ok := clientCapabilitiesFrom(ctx)
	if !ok {
		return run.Capabilities{}, nil
	}

	advertised := s.capabilities().Features

	var capabilities run.Capabilities
	for key, preference := range caps.Features {
		if !preference.Enabled {
			continue
		}
		published, known := protocol.LookupFeature(key)
		if !known || !advertised[key].Enabled {
			return run.Capabilities{}, NewCapabilityGapError(protocol.CapabilityRequirement{
				Type: protocol.RequirementFeature, Name: key,
			})
		}
		if published.RequiredByRun {
			switch key {
			case protocol.FeatureChildRuns:
				capabilities.ChildRuns = true
			default:
				return run.Capabilities{}, fmt.Errorf(
					"delivery: required Run protocol feature %q has no application policy mapping",
					key,
				)
			}
		}
	}

	for _, declared := range caps.InterruptTypes {
		kind, backed := interruptTypeFromWire(declared)
		if backed {
			capabilities.InterruptTypes = append(capabilities.InterruptTypes, kind)
			continue
		}
		return run.Capabilities{}, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown interruptTypes value %q", declared))
	}
	return capabilities.Normalized(), nil
}

func (s *Handler) missingFeatureRequirements(
	ctx context.Context,
	required ...string,
) []protocol.CapabilityRequirement {
	var client *protocol.ClientCapabilities
	if declared, ok := clientCapabilitiesFrom(ctx); ok {
		client = declared
	}
	return protocol.MissingFeatureRequirements(
		s.capabilities().Features, client, required...,
	)
}

func (s *Handler) requireFeature(ctx context.Context, feature string) error {
	missing := s.missingFeatureRequirements(ctx, feature)
	if len(missing) == 0 {
		return nil
	}
	return NewCapabilityGapError(missing...)
}

func (s *Handler) requestCanUseFeature(ctx context.Context, feature string) bool {
	return len(s.missingFeatureRequirements(ctx, feature)) == 0
}

func capabilityGap(missing run.Capabilities) *CapabilityGapError {
	requirements := make([]protocol.CapabilityRequirement, 0,
		1+len(missing.InterruptTypes))
	if missing.ChildRuns {
		requirements = append(requirements, protocol.CapabilityRequirement{
			Type: protocol.RequirementFeature, Name: protocol.FeatureChildRuns,
		})
	}
	for _, kind := range missing.InterruptTypes {
		requirements = append(requirements, protocol.CapabilityRequirement{
			Type: protocol.RequirementInterruptType, Name: string(presentInterruptType(kind)),
		})
	}
	return NewCapabilityGapError(requirements...)
}

func interruptTypeFromWire(kind protocol.InterruptType) (run.InterruptType, bool) {
	switch kind {
	case protocol.InterruptApproval:
		return run.InterruptApproval, true
	case protocol.InterruptQuestion:
		return run.InterruptQuestion, true
	default:
		return "", false
	}
}

func presentInterruptType(kind run.InterruptType) protocol.InterruptType {
	switch kind {
	case run.InterruptApproval:
		return protocol.InterruptApproval
	case run.InterruptQuestion:
		return protocol.InterruptQuestion
	default:
		panic("delivery: unknown interrupt kind")
	}
}
