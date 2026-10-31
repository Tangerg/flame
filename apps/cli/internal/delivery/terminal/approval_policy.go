package terminal

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/layout"
)

// approvalScreens chooses the Runtime approval mode and manages remembered
// approval rules; observing reports whether a Run is being followed, during
// which the mode cannot change.
type approvalScreens struct {
	screenHost
	service    ApprovalPolicy
	observing  func() bool
	modePicker *picker[protocol.ApprovalModePolicy]
	modeDialog *presentationDialog
}

func newApprovalScreens(host screenHost, service ApprovalPolicy, observing func() bool) *approvalScreens {
	p := &approvalScreens{screenHost: host, service: service, observing: observing}
	p.modePicker = newPicker(host.theme, host.glyphs, "search approval modes",
		func(policy protocol.ApprovalModePolicy) string { return approvalModeTitle(policy.Mode) },
		approvalPolicyDetail,
		func(policy protocol.ApprovalModePolicy) {
			if !p.modeDialog.Open() {
				return
			}
			p.modeDialog.Dismiss()
			p.setApprovalMode(policy.Mode)
		},
	)
	p.modeDialog = newPresentationDialog(kit.DialogConfig{
		Stack: host.stack, Theme: host.theme, Glyphs: host.glyphs, Title: "Runtime approval mode", Body: p.modePicker,
		Where: layout.Placement{Width: 88, Height: 9},
	})
	p.modePicker.cancel = p.modeDialog.Dismiss
	return p
}

func (p *approvalScreens) ChooseApprovalMode() {
	if p.observing() {
		p.message("approval mode changes apply between runs")
		return
	}
	p.tasks.session(pickerCatalogOperation, true,
		func(ctx context.Context) (protocol.ApprovalModeResult, error) { return p.service.GetApprovalMode(ctx) },
		func(result protocol.ApprovalModeResult, err error) {
			if err != nil {
				p.message("could not read approval modes: " + err.Error())
				return
			}
			p.modePicker.Reset()
			p.modePicker.SetItems(result.Modes)
			p.modeDialog.Show()
			p.status.note("choose the runtime approval mode")
		},
	)
}

func (p *approvalScreens) setApprovalMode(mode protocol.ApprovalMode) {
	p.tasks.admissionMutation(approvalModeOperation, true,
		func(ctx context.Context) (protocol.ApprovalModeResult, error) {
			return p.service.SetApprovalMode(ctx, mode)
		},
		func(applied protocol.ApprovalModeResult, err error) {
			if err != nil {
				p.message("could not set approval mode: " + err.Error())
				return
			}
			p.message("approval mode · " + string(applied.Mode))
		},
	)
}

func (p *approvalScreens) ShowApprovalRules() {
	p.reader.show(p.approvalRulesReaderQuery())
}

func (p *approvalScreens) approvalRulesReaderQuery() *readerQuery {
	sessionID := p.session.current.ID
	return &readerQuery{
		status: "loading approval rules", follows: followsTopic(protocol.TopicApprovalsChanged, protocol.TopicMCPChanged),
		read: func(ctx context.Context) (readerDocument, error) {
			rules, err := p.service.ListApprovalRules(ctx, sessionID)
			if err != nil {
				return readerDocument{}, err
			}
			return approvalRulesDocument(rules)
		},
	}
}

func (p *approvalScreens) PrepareDeleteApprovalRule(identity string) error {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return errors.New("usage: /rule-delete <rule-id>")
	}
	sessionID := p.session.current.ID
	p.status.note("loading approval rule to forget")
	if !p.tasks.session(approvalRuleOperation, false,
		func(ctx context.Context) (protocol.ApprovalRule, error) {
			rules, err := p.service.ListApprovalRules(ctx, sessionID)
			if err != nil {
				return protocol.ApprovalRule{}, err
			}
			return resolveApprovalRule(rules, identity)
		},
		func(rule protocol.ApprovalRule, err error) {
			if err != nil {
				p.message("load approval rule failed: " + err.Error())
				return
			}
			subject := conversation.ApprovalSubject(rule.Subject)
			p.confirm(
				"Forget approval rule",
				"Forget "+rule.ID+" ("+rule.ModelName+":"+subject+")?",
				"Forget permanently",
				"",
				func() { p.deleteApprovalRule(sessionID, rule.ID) },
			)
		},
	) {
		return errors.New("another approval operation is running")
	}
	return nil
}

func (p *approvalScreens) deleteApprovalRule(sessionID, id string) {
	p.status.note("forgetting approval rule " + id)
	if !p.tasks.admissionMutation(approvalRuleOperation, false,
		func(ctx context.Context) (approvalRuleDeletionResult, error) {
			if err := p.service.DeleteApprovalRule(ctx, id); err != nil {
				return approvalRuleDeletionResult{}, err
			}
			rules, err := p.service.ListApprovalRules(ctx, sessionID)
			return approvalRuleDeletionResult{id: id, rules: rules}, err
		},
		func(deleted approvalRuleDeletionResult, err error) {
			if err != nil {
				p.message("forget approval rule failed: " + err.Error())
				return
			}
			p.status.note("approval rule forgotten · " + deleted.id)
			if p.session.current.ID == sessionID {
				document, err := approvalRulesDocument(deleted.rules)
				if err != nil {
					p.message("forget approval rule: " + err.Error())
					return
				}
				p.reader.open(p.approvalRulesReaderQuery(), document)
			}
		},
	) {
		p.message("another approval operation is running")
	}
}

func (p *approvalScreens) setApprovalRule(request protocol.SetApprovalRuleRequest) error {
	source, err := mcp.ToolLabel(request.Tool)
	if err != nil {
		return err
	}
	label := "remembering global " + string(request.Decision) + " for " + source
	p.status.note(label)
	if !p.tasks.admissionMutation(approvalRuleOperation, false,
		func(ctx context.Context) (struct{}, error) {
			return struct{}{}, p.service.SetApprovalRule(ctx, request)
		},
		func(_ struct{}, err error) {
			if err != nil {
				p.message(label + " failed: " + err.Error())
				return
			}
			p.message(label + " accepted")
		},
	) {
		return errors.New("another approval operation is running")
	}
	return nil
}
