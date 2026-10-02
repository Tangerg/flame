package runtimefixture

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

func (r *Runtime) ListApprovalRules(ctx context.Context, sessionID string) ([]protocol.ApprovalRule, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	if err := (protocol.ListApprovalRulesRequest{SessionID: sessionID}).ValidateWire(); err != nil {
		return nil, fmt.Errorf("list approval rules: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	projectDir := ""
	if sessionID != "" {
		session := r.sessions[sessionID]
		if session == nil {
			return nil, fmt.Errorf("%w: %s", conversation.ErrSessionNotFound, sessionID)
		}
		projectDir = session.meta.Workspace.ProjectRoot
	}
	out := make([]protocol.ApprovalRule, 0, len(r.rules))
	for _, stored := range r.rules {
		if ruleApplies(stored, sessionID, projectDir) {
			out = append(out, stored.view)
		}
	}
	slices.Reverse(out)
	return out, nil
}

func (r *Runtime) DeleteApprovalRule(ctx context.Context, id string) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("delete approval rule: id is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	at := slices.IndexFunc(r.rules, func(rule storedRule) bool { return rule.view.ID == id })
	if at >= 0 {
		r.rules = slices.Delete(r.rules, at, at+1)
	}
	return nil
}

func (r *Runtime) rememberApprovalLocked(run *runState, approval conversation.Approval, answer conversation.ApprovalAnswer) {
	if !approval.Rememberable || answer.Remember == "" {
		return
	}
	session := r.sessions[run.sessionID]
	tool, subject := approvalRuleParts(approval)
	scope := approvalRuleScope(answer.Remember)
	for _, stored := range r.rules {
		rule := stored.view
		if rule.Tool == (protocol.ToolRef{Type: protocol.ToolRefBuiltIn, Name: tool}) && rule.Subject == (protocol.ApprovalSubject{Type: protocol.ApprovalSubjectExact, Value: subject}) && rule.Scope == scope && ruleApplies(stored, run.sessionID, session.meta.Workspace.ProjectRoot) {
			return
		}
	}
	rule := protocol.ApprovalRule{
		ID: r.identities.next(ruleIdentity), Scope: scope,
		Tool: protocol.ToolRef{Type: protocol.ToolRefBuiltIn, Name: tool}, ModelName: tool, Subject: protocol.ApprovalSubject{Type: protocol.ApprovalSubjectExact, Value: subject}, Decision: approvalRuleDecision(answer.Decision),
	}
	stored := storedRule{view: rule}
	switch answer.Remember {
	case protocol.RememberSession:
		stored.sessionID = run.sessionID
	case protocol.RememberProject:
		stored.view.Dir = session.meta.Workspace.ProjectRoot
	case protocol.RememberGlobal:
	default:
		return
	}
	r.rules = append(r.rules, stored)
}

func (r *Runtime) resolveRememberedLocked(run *runState, interactions []conversation.Interaction) (resolved []conversation.InterruptAnswer, pending []conversation.Interaction) {
	resolved = make([]conversation.InterruptAnswer, 0, len(interactions))
	pending = make([]conversation.Interaction, 0, len(interactions))
	for _, interaction := range interactions {
		approval, ok := interaction.(conversation.Approval)
		if !ok {
			pending = append(pending, conversation.CloneInteraction(interaction))
			continue
		}
		answer, matched := r.rememberedAnswerLocked(run, approval)
		if !matched {
			pending = append(pending, conversation.CloneInteraction(interaction))
			continue
		}
		resolved = append(resolved, conversation.InterruptAnswer{ItemID: approval.ItemID, Answer: answer})
	}
	return resolved, pending
}

func (r *Runtime) rememberedAnswerLocked(run *runState, approval conversation.Approval) (conversation.ApprovalAnswer, bool) {
	workspace := r.sessions[run.sessionID].meta.Workspace.ProjectRoot
	tool, subject := approvalRuleParts(approval)
	for _, stored := range slices.Backward(r.rules) {
		rule := stored.view
		if rule.Tool == (protocol.ToolRef{Type: protocol.ToolRefBuiltIn, Name: tool}) && rule.Subject == (protocol.ApprovalSubject{Type: protocol.ApprovalSubjectExact, Value: subject}) && ruleApplies(stored, run.sessionID, workspace) {
			return conversation.ApprovalAnswer{Decision: approvalDecision(rule.Decision), Remember: rememberScope(rule.Scope)}, true
		}
	}
	return conversation.ApprovalAnswer{}, false
}

func approvalRuleDecision(decision protocol.ApprovalDecision) protocol.ApprovalRuleDecision {
	if decision == protocol.ApprovalApprove {
		return protocol.ApprovalRuleDecisionAllow
	}
	return protocol.ApprovalRuleDecisionDeny
}

func approvalDecision(decision protocol.ApprovalRuleDecision) protocol.ApprovalDecision {
	if decision == protocol.ApprovalRuleDecisionAllow {
		return protocol.ApprovalApprove
	}
	return protocol.ApprovalDeny
}

func ruleApplies(rule storedRule, sessionID, workspace string) bool {
	switch rule.view.Scope {
	case protocol.ApprovalRuleScopeSession:
		return rule.sessionID == sessionID
	case protocol.ApprovalRuleScopeProject:
		return rule.view.Dir == workspace
	case protocol.ApprovalRuleScopeGlobal:
		return true
	default:
		return false
	}
}

func approvalRuleScope(scope protocol.RememberScopeKind) protocol.ApprovalRuleScope {
	switch scope {
	case protocol.RememberSession:
		return protocol.ApprovalRuleScopeSession
	case protocol.RememberProject:
		return protocol.ApprovalRuleScopeProject
	case protocol.RememberGlobal:
		return protocol.ApprovalRuleScopeGlobal
	default:
		return ""
	}
}

func rememberScope(scope protocol.ApprovalRuleScope) protocol.RememberScopeKind {
	switch scope {
	case protocol.ApprovalRuleScopeSession:
		return protocol.RememberSession
	case protocol.ApprovalRuleScopeProject:
		return protocol.RememberProject
	case protocol.ApprovalRuleScopeGlobal:
		return protocol.RememberGlobal
	default:
		return ""
	}
}

func approvalRuleParts(approval conversation.Approval) (tool, subject string) {
	hint := strings.TrimSpace(approval.RuleHint)
	if hint != "" {
		if hintTool, hintSubject, ok := strings.Cut(hint, ":"); ok && strings.TrimSpace(hintTool) != "" {
			return strings.TrimSpace(hintTool), strings.TrimSpace(hintSubject)
		}
	}
	if approval.Tool == nil {
		return "unknown", strings.TrimSpace(approval.Title)
	}
	tool = strings.TrimSpace(approval.Tool.Name)
	if tool == "" {
		tool = string(approval.Tool.Kind)
	}
	switch approval.Tool.Kind {
	case conversation.ToolShell:
		subject = approval.Tool.Command
	case conversation.ToolEdit, conversation.ToolRead:
		subject = approval.Tool.Path
	case conversation.ToolSearch:
		subject = approval.Tool.Query
	case conversation.ToolWeb:
		subject = approval.Tool.URL
	case conversation.ToolUnknown, conversation.ToolTask:
	}
	if strings.TrimSpace(subject) == "" {
		subject = approval.Tool.Summary
	}
	return tool, strings.TrimSpace(subject)
}

func (r *Runtime) SetApprovalRule(ctx context.Context, request protocol.SetApprovalRuleRequest) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if err := protocol.ValidateWireTree(request); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	name, found := r.ToolModelNames[request.Tool]
	if !found {
		return fmt.Errorf("fixture: tool source has no configured model name: %v", request.Tool)
	}
	projectDir := ""
	if request.Scope == protocol.ApprovalRuleScopeProject {
		session := r.sessions[request.SessionID]
		if session == nil {
			return conversation.ErrSessionNotFound
		}
		projectDir = session.meta.Workspace.ProjectRoot
	}
	for i := range r.rules {
		stored := &r.rules[i]
		if stored.view.Tool == request.Tool && stored.view.Scope == request.Scope && stored.view.Subject == request.Subject && stored.view.Dir == projectDir && stored.sessionID == request.SessionID {
			stored.view.Decision = request.Decision
			stored.view.ModelName = name
			return nil
		}
	}
	r.rules = append(r.rules, storedRule{view: protocol.ApprovalRule{ID: r.identities.next(ruleIdentity), Tool: request.Tool, ModelName: name, Scope: request.Scope, Dir: projectDir, Subject: request.Subject, Decision: request.Decision}, sessionID: request.SessionID})
	return nil
}
