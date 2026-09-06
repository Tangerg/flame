package delivery

import (
	"context"
	"fmt"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/protocol"
)

// Capability negotiation: what a request is entitled to, resolved once.
//
// runs.start freezes the answer onto the new Run as its capabilities;
// runs.resume and runs.subscribe hand the same answer to the application, which
// refuses a caller that cannot cover what the Run already publishes. Both readings
// come from this one function, so "what you may ask for" and "what you must be
// able to follow" can never become two vocabularies that disagree.

// negotiateCapabilities resolves what the caller declared against what this build
// advertises.
//
// A declared capability this build cannot honor is a refusal, never a silent drop.
// Dropping it is the failure mode the contract names explicitly: a client that
// asked for subagents and received an ordinary run would fold a stream believing
// child events could appear in it, and a client whose interrupt type was quietly
// discarded would be handed a wait it never said it could answer.
//
// Absent capabilities select the Minimal Profile, which supports sending a
// message, watching the reply, and reloading history.
func (s *Handler) negotiateCapabilities(ctx context.Context) (run.Capabilities, error) {
	caps, ok := ClientCapabilitiesFrom(ctx)
	if !ok {
		return run.Capabilities{}, nil
	}

	advertised := s.capabilities().Features

	var capabilities run.Capabilities
	for key, preference := range caps.Features {
		if !preference.Enabled {
			// Declining a feature is always honorable, including one this build has
			// never heard of.
			continue
		}
		published, known := protocol.LookupFeature(key)
		if !known || !advertised[key].Enabled {
			return run.Capabilities{}, NewCapabilityGapError(protocol.CapabilityRequirement{
				Type: protocol.RequirementFeature, Name: key,
			})
		}
		// Only a feature that changes what the Run PUBLISHES belongs on the Run: the
		// capability set exists so a later subscriber can be told what it must understand,
		// and a feature invisible in the stream demands nothing of it.
		if published.RequiredByRunProtocol {
			switch key {
			case protocol.FeatureSubagents:
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
		kind, backed := interruptKindFromWire(declared)
		if backed {
			capabilities.InterruptKinds = append(capabilities.InterruptKinds, kind)
			continue
		}
		return run.Capabilities{}, fmt.Errorf("%w: unknown interruptTypes value %q", protocol.ErrInvalidParams, declared)
	}
	return capabilities.Normalized(), nil
}

// missingFeatureRequirements is the Delivery entry point for a gate whose
// trigger depends on durable state and therefore cannot live in MethodMeta.When.
// The actual Runtime/client decision is shared with the static dispatcher gate.
func (s *Handler) missingFeatureRequirements(
	ctx context.Context,
	required ...string,
) []protocol.CapabilityRequirement {
	var client *protocol.ClientCapabilities
	if declared, ok := ClientCapabilitiesFrom(ctx); ok {
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

// capabilityGap maps the complete missing semantic set to protocol requirements.
//
// Every gap at once, because a caller told about one at a time cannot get itself into
// a state where the call succeeds.
func capabilityGap(missing run.Capabilities) *CapabilityGapError {
	requirements := make([]protocol.CapabilityRequirement, 0,
		1+len(missing.InterruptKinds))
	if missing.ChildRuns {
		requirements = append(requirements, protocol.CapabilityRequirement{
			Type: protocol.RequirementFeature, Name: protocol.FeatureSubagents,
		})
	}
	for _, kind := range missing.InterruptKinds {
		requirements = append(requirements, protocol.CapabilityRequirement{
			Type: protocol.RequirementInterruptType, Name: string(presentInterruptType(kind)),
		})
	}
	return NewCapabilityGapError(requirements...)
}

// interruptKindFromWire maps a declared interrupt type onto the durable kind the
// runtime raises. It reports false for a type no kind backs — which is not the
// same as an unknown value, and the caller distinguishes them.
func interruptKindFromWire(kind protocol.InterruptType) (interrupt.Kind, bool) {
	switch kind {
	case protocol.InterruptApproval:
		return interrupt.Approval, true
	case protocol.InterruptQuestion:
		return interrupt.Question, true
	default:
		return "", false
	}
}

// presentInterruptType is the same mapping read outward, for a protocol profile or an
// interrupt on its way to the wire.
func presentInterruptType(kind interrupt.Kind) protocol.InterruptType {
	switch kind {
	case interrupt.Approval:
		return protocol.InterruptApproval
	case interrupt.Question:
		return protocol.InterruptQuestion
	default:
		panic("delivery: unknown interrupt kind")
	}
}
