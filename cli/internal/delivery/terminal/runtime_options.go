package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/flame/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func (a *app) buildRuntimePickers(theme kit.Theme, glyphs kit.Glyphs) {
	a.dialogs.modelPicker = newPicker(theme, glyphs, "search models",
		func(model protocol.Model) string {
			label := model.DisplayName
			if label == "" {
				label = model.ID
			}
			if model.Deprecated {
				label += " · deprecated"
			}
			return label
		},
		func(model protocol.Model) string { return model.Provider + "/" + model.ID },
		func(model protocol.Model) {
			if !a.dialogs.modelDialog.Open() {
				return
			}
			a.dialogs.modelDialog.Dismiss()
			a.selectSessionModel(model)
		},
	)
	a.dialogs.modelDialog = newPresentationDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Models", Body: a.dialogs.modelPicker,
		Where: layout.Placement{Width: 76, Height: 14},
	})
	a.dialogs.modelPicker.cancel = a.dialogs.modelDialog.Dismiss

	a.dialogs.approvalModePicker = newPicker(theme, glyphs, "search approval modes",
		approvalModeTitle,
		approvalModeDetail,
		func(mode protocol.ApprovalMode) {
			if !a.dialogs.approvalModeDialog.Open() {
				return
			}
			a.dialogs.approvalModeDialog.Dismiss()
			a.setApprovalMode(mode)
		},
	)
	a.dialogs.approvalModePicker.SetItems([]protocol.ApprovalMode{protocol.ApprovalModeSafe, protocol.ApprovalModeBalanced, protocol.ApprovalModeYolo})
	a.dialogs.approvalModeDialog = newPresentationDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Runtime approval mode", Body: a.dialogs.approvalModePicker,
		Where: layout.Placement{Width: 88, Height: 9},
	})
	a.dialogs.approvalModePicker.cancel = a.dialogs.approvalModeDialog.Dismiss
}

func (a *app) selectSessionModel(model protocol.Model) {
	sessionID := a.session.current.ID
	a.runSessionChange("selecting model",
		func(ctx context.Context) (agent.Session, error) {
			latest, err := a.runtime.GetSession(ctx, sessionID)
			if err != nil {
				return agent.Session{}, err
			}
			return session.Update(ctx, a.runtime, agent.UpdateSession{
				SessionID:        sessionID,
				Model:            &agent.ModelRef{Provider: model.Provider, Model: model.ID},
				ExpectedRevision: latest.Session.Revision,
			})
		},
		func(updated agent.Session) error {
			a.setActiveSession(updated)
			a.options.Provider, a.options.Model = model.Provider, model.ID
			a.syncOptions("model · " + model.Provider + "/" + model.ID)
			return nil
		},
	)
}

func (a *app) ChooseModel() {
	if a.execution.observing() {
		a.message("model changes apply between runs")
		return
	}
	a.message("loading models")
	a.loadModelPicker(true)
}

func (a *app) loadModelPicker(reset bool) {
	a.runOperation(pickerCatalogOperation, true,
		func(ctx context.Context) ([]protocol.Model, error) { return a.runtime.ListModels(ctx) },
		func(models []protocol.Model, err error) {
			if err != nil {
				a.message("could not load models: " + err.Error())
				return
			}
			if reset {
				a.dialogs.modelPicker.Reset()
			}
			a.dialogs.modelPicker.SetItems(models)
			a.dialogs.modelDialog.Show()
			a.status.note("choose a provider-qualified model")
		},
	)
}

func (a *app) ChooseApprovalMode() {
	if a.execution.observing() {
		a.message("approval mode changes apply between runs")
		return
	}
	a.dialogs.approvalModePicker.Reset()
	a.dialogs.approvalModeDialog.Show()
	a.status.note("choose the runtime approval mode")
}

func (a *app) setApprovalMode(mode protocol.ApprovalMode) {
	a.runAdmissionMutation(approvalModeOperation, true,
		func(ctx context.Context) (protocol.ApprovalMode, error) { return a.runtime.SetApprovalMode(ctx, mode) },
		func(applied protocol.ApprovalMode, err error) {
			if err != nil {
				a.message("could not set approval mode: " + err.Error())
				return
			}
			a.message("approval mode · " + string(applied))
		},
	)
}

func (a *app) ShowRuntimeStatus() {
	a.runOperation(pickerCatalogOperation, true,
		func(ctx context.Context) (protocol.ApprovalMode, error) { return a.runtime.GetApprovalMode(ctx) },
		func(mode protocol.ApprovalMode, err error) {
			if err != nil {
				a.message("could not read runtime status: " + err.Error())
				return
			}
			a.transcript.Append(&kit.Entry{
				Theme: a.transcript.theme, Label: "runtime options",
				Body: runtimeStatusText(a.runtimeProfile, a.displayOptions(), mode),
			})
		},
	)
}

func runtimeStatusText(profile *runtimebinding.Profile, options agent.RunOptions, mode protocol.ApprovalMode) string {
	lines := []string{
		"model: " + modelLabel(options),
		"approval mode: " + string(mode) + limitsLabel(options.Limits),
	}
	if profile == nil {
		return strings.Join(lines, "\n")
	}
	features := profile.AvailableFeatureNames()
	if len(features) == 0 {
		features = []string{"none"}
	}
	limits := profile.Limits
	profileLines := []string{
		fmt.Sprintf("runtime: %s %s", profile.Server.Name, profile.Server.Version),
		"protocol: " + profile.Protocol.Version,
		"default workspace: " + profile.Server.DefaultWorkspace,
		"home: " + profile.Server.Home,
		"available features: " + strings.Join(features, ", "),
		fmt.Sprintf("run concurrency: %s", runConcurrencyLabel(limits.RunConcurrency)),
		fmt.Sprintf("run replay: %d events / %s / %s", limits.RunReplay.MaxEvents, formatRuntimeBytes(limits.RunReplay.MaxBytes), limits.RunReplay.Scope),
		"command replay retention: " + formatRuntimeSeconds(int(limits.CommandReplay.Retention()/time.Second)),
		"MCP authorization retention: " + formatRuntimeSeconds(limits.MCPAuthorizationRetentionSeconds),
		fmt.Sprintf("runtime subscriptions: %d topics / %d watches", limits.RuntimeSubscription.MaxTopics, limits.RuntimeSubscription.MaxWatches),
		fmt.Sprintf("surface: %d run events / %d topics / %d streaming methods", len(profile.RunEvents), len(profile.RuntimeTopics), len(profile.StreamingMethods)),
	}
	return strings.Join(slices.Concat(profileLines, lines), "\n")
}

func runConcurrencyLabel(limit runtimebinding.RunConcurrencyLimit) string {
	maximum, bounded := limit.Maximum()
	if !bounded {
		return "unbounded"
	}
	return fmt.Sprintf("at most %d runs", maximum)
}

func formatRuntimeSeconds(value int) string {
	if value%3600 == 0 {
		return fmt.Sprintf("%dh", value/3600)
	}
	if value%60 == 0 {
		return fmt.Sprintf("%dm", value/60)
	}
	return fmt.Sprintf("%ds", value)
}

func formatRuntimeBytes(value int) string {
	const unit = 1024
	switch {
	case value >= unit*unit && value%(unit*unit) == 0:
		return fmt.Sprintf("%d MiB", value/(unit*unit))
	case value >= unit && value%unit == 0:
		return fmt.Sprintf("%d KiB", value/unit)
	default:
		return fmt.Sprintf("%d B", value)
	}
}

func (a *app) ShowApprovalRules() {
	a.executeRuntimeReaderQuery(a.approvalRulesReaderQuery())
}

func (a *app) approvalRulesReaderQuery() runtimeReaderQuery {
	sessionID := a.session.current.ID
	return runtimeReaderQuery{
		status: "loading approval rules", mode: runtimeReaderApprovalRules,
		read: func(ctx context.Context) (readerDocument, error) {
			rules, err := a.runtime.ListApprovalRules(ctx, sessionID)
			if err != nil {
				return readerDocument{}, err
			}
			return approvalRulesDocument(rules), nil
		},
	}
}

func approvalRulesDocument(rules []protocol.ApprovalRule) readerDocument {
	if len(rules) == 0 {
		return paragraphDocument("Approval rules", "none remembered", []string{"No remembered approval rules."})
	}
	lines := make([]string, 0, len(rules))
	for _, rule := range rules {
		subject := rule.Subject
		if subject == "" {
			subject = "*"
		}
		lines = append(lines, fmt.Sprintf("%s  %s  %s  %s:%s", rule.ID, rule.Scope, rule.Decision, rule.Tool, subject))
	}
	return paragraphDocument("Approval rules", fmt.Sprintf("%d remembered", len(rules)), lines)
}

func (a *app) PrepareDeleteApprovalRule(identity string) error {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return errors.New("usage: /rule-delete <rule-id>")
	}
	sessionID := a.session.current.ID
	a.status.note("loading approval rule to forget")
	if !a.runOperation(approvalRuleOperation, false,
		func(ctx context.Context) (protocol.ApprovalRule, error) {
			rules, err := a.runtime.ListApprovalRules(ctx, sessionID)
			if err != nil {
				return protocol.ApprovalRule{}, err
			}
			return resolveApprovalRule(rules, identity)
		},
		func(rule protocol.ApprovalRule, err error) {
			if err != nil {
				a.message("load approval rule failed: " + err.Error())
				return
			}
			subject := rule.Subject
			if subject == "" {
				subject = "*"
			}
			a.confirmAction(
				"Forget approval rule",
				"Forget "+rule.ID+" ("+rule.Tool+":"+subject+")?",
				"Forget permanently",
				func() { a.deleteApprovalRule(sessionID, rule.ID) },
			)
		},
	) {
		return errors.New("another approval operation is running")
	}
	return nil
}

func resolveApprovalRule(rules []protocol.ApprovalRule, identity string) (protocol.ApprovalRule, error) {
	for _, rule := range rules {
		if rule.ID == identity {
			return rule, nil
		}
	}
	var matches []protocol.ApprovalRule
	for _, rule := range rules {
		if strings.HasPrefix(rule.ID, identity) {
			matches = append(matches, rule)
		}
	}
	switch len(matches) {
	case 0:
		return protocol.ApprovalRule{}, errors.New("approval rule not found: " + identity)
	case 1:
		return matches[0], nil
	default:
		return protocol.ApprovalRule{}, errors.New("approval rule identity is ambiguous; use the full id")
	}
}

type approvalRuleDeletionResult struct {
	id    string
	rules []protocol.ApprovalRule
}

func (a *app) deleteApprovalRule(sessionID, id string) {
	a.status.note("forgetting approval rule " + id)
	if !a.runAdmissionMutation(approvalRuleOperation, false,
		func(ctx context.Context) (approvalRuleDeletionResult, error) {
			if err := a.runtime.DeleteApprovalRule(ctx, id); err != nil {
				return approvalRuleDeletionResult{}, err
			}
			rules, err := a.runtime.ListApprovalRules(ctx, sessionID)
			if err != nil {
				return approvalRuleDeletionResult{}, err
			}
			if err := validateApprovalRuleDeletion(rules, id); err != nil {
				return approvalRuleDeletionResult{}, fmt.Errorf("verify approval rule deletion: %w", err)
			}
			return approvalRuleDeletionResult{id: id, rules: rules}, nil
		},
		func(deleted approvalRuleDeletionResult, err error) {
			if err != nil {
				a.message("forget approval rule failed: " + err.Error())
				return
			}
			a.status.note("approval rule forgotten · " + deleted.id)
			if a.session.current.ID == sessionID {
				a.setRuntimeReader(runtimeReaderApprovalRules)
				a.openReaderDocument(approvalRulesDocument(deleted.rules))
			}
		},
	) {
		a.message("another approval operation is running")
	}
}

func (a *app) syncOptions(message string) {
	a.prompt.SetOptions(a.displayOptions())
	a.brand.SetOptions(a.displayOptions())
	a.message(message)
}

func validateApprovalRuleDeletion(rules []protocol.ApprovalRule, id string) error {
	for _, rule := range rules {
		if rule.ID == id {
			return fmt.Errorf("approval rule %q remains after deletion", id)
		}
	}
	return nil
}

func approvalModeTitle(mode protocol.ApprovalMode) string {
	switch mode {
	case protocol.ApprovalModeSafe:
		return "Safe"
	case protocol.ApprovalModeBalanced:
		return "Balanced"
	case protocol.ApprovalModeYolo:
		return "Yolo"
	default:
		return string(mode)
	}
}

func approvalModeDetail(mode protocol.ApprovalMode) string {
	switch mode {
	case protocol.ApprovalModeSafe:
		return "ask before write, exec, and network tools"
	case protocol.ApprovalModeBalanced:
		return "allow writes and network; ask before shell execution"
	case protocol.ApprovalModeYolo:
		return "allow every tool without approval prompts"
	default:
		return ""
	}
}
