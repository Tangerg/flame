package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func (s *Handler) GetApprovalMode(ctx context.Context) (*protocol.ApprovalModeResult, error) {
	m, err := s.approvals.DefaultMode(ctx)
	if err != nil {
		return nil, err
	}
	mode, ok := presentApprovalMode(m)
	if !ok {
		return nil, fmt.Errorf("delivery: %w: %q", approval.ErrInvalidMode, m)
	}
	return &protocol.ApprovalModeResult{Mode: mode, Modes: presentApprovalModePolicies()}, nil
}

func (s *Handler) SetApprovalMode(ctx context.Context, in protocol.SetApprovalModeRequest) (*protocol.ApprovalModeResult, error) {
	mode, ok := approvalModeFromWire(in.Mode)
	if !ok {
		return nil, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown approval mode %q", in.Mode))
	}
	if err := s.approvals.SetDefaultMode(ctx, mode); err != nil {
		return nil, err
	}
	return &protocol.ApprovalModeResult{Mode: in.Mode, Modes: presentApprovalModePolicies()}, nil
}

func (s *Handler) ListApprovalRules(ctx context.Context, in protocol.ListApprovalRulesRequest) (*protocol.ListApprovalRulesResult, error) {
	sessionID, parseErr := optionalSessionParameter(in.SessionID)
	if parseErr != nil {
		return nil, parseErr
	}
	rules, err := s.approvals.ListRules(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.ApprovalRule, 0, len(rules))
	for _, r := range rules {
		wire, err := presentApprovalRule(r)
		if err != nil {
			return nil, err
		}
		out = append(out, wire)
	}
	return &protocol.ListApprovalRulesResult{Rules: out}, nil
}

func (s *Handler) ForgetApprovalRule(ctx context.Context, in protocol.ForgetApprovalRuleRequest) error {
	return s.approvals.ForgetRule(ctx, in.ID)
}

func presentApprovalRule(r approvals.RuleView) (protocol.ApprovalRule, error) {
	scope, ok := approvalScopes.wire(r.Scope)
	if !ok {
		return protocol.ApprovalRule{}, fmt.Errorf("approval.listRules: unsupported scope %q", r.Scope)
	}
	decision, ok := approvalDecisions.wire(r.Decision)
	if !ok {
		return protocol.ApprovalRule{}, fmt.Errorf("approval.listRules: unsupported decision %q", r.Decision)
	}
	ref, err := presentToolRef(r.Tool)
	if err != nil {
		return protocol.ApprovalRule{}, err
	}
	wire := protocol.ApprovalRule{
		ID:    r.ID,
		Scope: scope,
		Tool:  ref, ModelName: r.Tool.ModelName(), Stale: r.Stale,
		Dir:      r.ProjectDir,
		Subject:  protocol.ApprovalSubject{Type: approvalSubjects.mustWire(r.Subject.Type), Value: r.Subject.Value},
		Decision: decision,
	}
	return wire, nil
}

var approvalScopes = wireVocabulary[approval.Scope, protocol.ApprovalRuleScope]{
	{approval.ScopeSession, protocol.ApprovalRuleScopeSession},
	{approval.ScopeProject, protocol.ApprovalRuleScopeProject},
	{approval.ScopeGlobal, protocol.ApprovalRuleScopeGlobal},
}

var approvalDecisions = wireVocabulary[approval.Decision, protocol.ApprovalRuleDecision]{
	{approval.Allow, protocol.ApprovalRuleDecisionAllow},
	{approval.Deny, protocol.ApprovalRuleDecisionDeny},
}

var approvalSubjects = wireVocabulary[approval.SubjectType, protocol.ApprovalSubjectType]{
	{approval.SubjectAll, protocol.ApprovalSubjectAll},
	{approval.SubjectExact, protocol.ApprovalSubjectExact},
	{approval.SubjectGlob, protocol.ApprovalSubjectGlob},
}

func presentApprovalModePolicies() []protocol.ApprovalModePolicy {
	modes := []approval.Mode{approval.ModeSafe, approval.ModeBalanced, approval.ModeYolo}
	out := make([]protocol.ApprovalModePolicy, 0, len(modes))
	for _, mode := range modes {
		wire, ok := presentApprovalMode(mode)
		if !ok {
			panic(fmt.Sprintf("delivery: approval mode %q has no wire name", mode))
		}
		out = append(out, protocol.ApprovalModePolicy{
			Mode:    wire,
			Write:   presentApprovalGate(approval.GateFor(tool.SafetyClassWrite, mode)),
			Exec:    presentApprovalGate(approval.GateFor(tool.SafetyClassExec, mode)),
			Network: presentApprovalGate(approval.GateFor(tool.SafetyClassNetwork, mode)),
		})
	}
	return out
}

func presentApprovalGate(gate approval.GateAction) protocol.ApprovalGate {
	switch gate {
	case approval.GatePass:
		return protocol.ApprovalGatePass
	case approval.GatePrompt:
		return protocol.ApprovalGatePrompt
	case approval.GateDeny:
		return protocol.ApprovalGateDeny
	default:
		panic(fmt.Sprintf("delivery: unknown approval gate %q", gate))
	}
}

func presentApprovalMode(m approval.Mode) (protocol.ApprovalMode, bool) {
	switch m {
	case approval.ModeSafe:
		return protocol.ApprovalModeSafe, true
	case approval.ModeBalanced:
		return protocol.ApprovalModeBalanced, true
	case approval.ModeYolo:
		return protocol.ApprovalModeYolo, true
	default:
		return "", false
	}
}

func rememberScopeFromWire(s protocol.RememberScopeKind) (approval.Scope, bool) {
	switch s {
	case protocol.RememberSession:
		return approval.ScopeSession, true
	case protocol.RememberProject:
		return approval.ScopeProject, true
	case protocol.RememberGlobal:
		return approval.ScopeGlobal, true
	}
	return "", false
}

func approvalModeFromWire(m protocol.ApprovalMode) (approval.Mode, bool) {
	switch m {
	case protocol.ApprovalModeSafe:
		return approval.ModeSafe, true
	case protocol.ApprovalModeBalanced:
		return approval.ModeBalanced, true
	case protocol.ApprovalModeYolo:
		return approval.ModeYolo, true
	}
	return "", false
}

func (s *Handler) SetApprovalRule(ctx context.Context, in protocol.SetApprovalRuleRequest) error {
	sessionID, parseErr := optionalSessionParameter(in.SessionID)
	if parseErr != nil {
		return parseErr
	}
	ref, err := toolRefFromWire(in.Tool)
	if err != nil {
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error())
	}
	scope, scopeKnown := approvalScopes.domain(in.Scope)
	decision, decisionKnown := approvalDecisions.domain(in.Decision)
	subject, subjectKnown := approvalSubjects.domain(in.Subject.Type)
	if !scopeKnown || !decisionKnown || !subjectKnown {
		return protocol.NewFailure(protocol.ErrInvalidParams, "invalid approval rule")
	}
	err = s.approvals.SetRule(ctx, approvals.RuleChange{Tool: ref, Scope: scope, SessionID: sessionID, Subject: approval.Subject{Type: subject, Value: in.Subject.Value}, Decision: decision})
	if errors.Is(err, approval.ErrInvalidRule) {
		return protocol.InvalidParameters(err)
	}
	return wireSessionErr(err)
}
