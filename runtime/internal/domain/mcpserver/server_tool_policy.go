package mcpserver

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
)

// ToolPolicyDecision is the complete durable policy vocabulary for one remote
// tool. Absence means the normal approval policy applies.
type ToolPolicyDecision string

const (
	ToolDisabled     ToolPolicyDecision = "disabled"
	ToolAutoApproved ToolPolicyDecision = "autoApproved"
)

// Valid reports whether decision is one of the durable policy states.
func (d ToolPolicyDecision) Valid() bool {
	return d == ToolDisabled || d == ToolAutoApproved
}

// ToolPolicyRule assigns one exact decision to one remote tool identity.
type ToolPolicyRule struct {
	Tool     RemoteToolName
	Decision ToolPolicyDecision
}

// ServerToolPolicy is one server's immutable, canonical policy aggregate.
// Rules are sorted by remote identity and a tool can have at most one decision,
// so contradictory disabled/auto-approved state is unrepresentable.
type ServerToolPolicy struct {
	rules []ToolPolicyRule
}

// ErrInvalidServerToolPolicy reports duplicate, contradictory, or malformed
// tool-policy material.
var ErrInvalidServerToolPolicy = errors.New("mcp: invalid server tool policy")

// NewServerToolPolicy builds a policy from the public two-list command shape.
func NewServerToolPolicy(disabled, autoApproved []RemoteToolName) (ServerToolPolicy, error) {
	if err := ValidateRemoteToolCount(len(disabled) + len(autoApproved)); err != nil {
		return ServerToolPolicy{}, fmt.Errorf("%w: %w", ErrInvalidServerToolPolicy, err)
	}
	rules := make([]ToolPolicyRule, 0, len(disabled)+len(autoApproved))
	for _, tool := range disabled {
		rules = append(rules, ToolPolicyRule{Tool: tool, Decision: ToolDisabled})
	}
	for _, tool := range autoApproved {
		rules = append(rules, ToolPolicyRule{Tool: tool, Decision: ToolAutoApproved})
	}
	return RestoreServerToolPolicy(rules)
}

// RestoreServerToolPolicy restores the normalized durable rule relation.
func RestoreServerToolPolicy(rules []ToolPolicyRule) (ServerToolPolicy, error) {
	if err := ValidateRemoteToolCount(len(rules)); err != nil {
		return ServerToolPolicy{}, fmt.Errorf("%w: %w", ErrInvalidServerToolPolicy, err)
	}
	canonical := slices.Clone(rules)
	slices.SortFunc(canonical, func(a, b ToolPolicyRule) int {
		return cmp.Compare(a.Tool.String(), b.Tool.String())
	})
	for i, rule := range canonical {
		if err := rule.Tool.Validate(); err != nil {
			return ServerToolPolicy{}, fmt.Errorf("%w: rule %d: %w", ErrInvalidServerToolPolicy, i, err)
		}
		if !rule.Decision.Valid() {
			return ServerToolPolicy{}, fmt.Errorf(
				"%w: tool %q has unknown decision %q",
				ErrInvalidServerToolPolicy,
				rule.Tool,
				rule.Decision,
			)
		}
		if i > 0 && canonical[i-1].Tool == rule.Tool {
			return ServerToolPolicy{}, fmt.Errorf(
				"%w: tool %q has more than one decision",
				ErrInvalidServerToolPolicy,
				rule.Tool,
			)
		}
	}
	return ServerToolPolicy{rules: canonical}, nil
}

// Validate reports whether the aggregate retains its canonical invariants.
func (p ServerToolPolicy) Validate() error {
	restored, err := RestoreServerToolPolicy(p.rules)
	if err != nil {
		return err
	}
	if !slices.Equal(restored.rules, p.rules) {
		return fmt.Errorf("%w: rules are not in canonical order", ErrInvalidServerToolPolicy)
	}
	return nil
}

// Rules returns an isolated canonical snapshot for persistence.
func (p ServerToolPolicy) Rules() []ToolPolicyRule { return slices.Clone(p.rules) }

// DisabledTools returns the exact remote identities hidden from the model.
func (p ServerToolPolicy) DisabledTools() []RemoteToolName {
	return p.toolsWithDecision(ToolDisabled)
}

// AutoApprovedTools returns the exact remote identities allowed to skip HITL.
func (p ServerToolPolicy) AutoApprovedTools() []RemoteToolName {
	return p.toolsWithDecision(ToolAutoApproved)
}

func (p ServerToolPolicy) toolsWithDecision(decision ToolPolicyDecision) []RemoteToolName {
	var names []RemoteToolName
	for _, rule := range p.rules {
		if rule.Decision == decision {
			names = append(names, rule.Tool)
		}
	}
	return names
}
