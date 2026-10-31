package terminal

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/layout"
)

func (a *app) buildModelPicker(theme kit.Theme, glyphs kit.Glyphs) {
	a.models = newPickerDialog(pickerDialogConfig[protocol.Model]{
		stack: &a.stack, theme: theme, glyphs: glyphs, title: "Models", search: "search models",
		where: layout.Placement{Width: 76, Height: 14},
		label: func(model protocol.Model) string {
			label := model.DisplayName
			if label == "" {
				label = model.ID
			}
			if model.Deprecated {
				label += " · deprecated"
			}
			return label
		},
		detail:    func(model protocol.Model) string { return model.Provider + "/" + model.ID },
		choose:    a.selectSessionModel,
		dismissed: func() { a.operations.Cancel(pickerCatalogOperation) },
	})
}

func (a *app) selectSessionModel(model protocol.Model) {
	sessionID := a.session.current.ID
	a.runSessionChange("selecting model",
		func(ctx context.Context) (conversation.Session, error) {
			latest, err := a.runtime.GetSession(ctx, sessionID)
			if err != nil {
				return conversation.Session{}, err
			}
			return a.runtime.UpdateSession(ctx, conversation.UpdateSession{
				SessionID:        sessionID,
				Model:            &conversation.ModelRef{Provider: model.Provider, Model: model.ID},
				ExpectedRevision: latest.Session.Revision,
			})
		},
		func(updated conversation.Session) error {
			// The Session now owns the choice. A retained launch override would
			// outrank it here and follow the user into every other Session.
			a.options.Provider, a.options.Model, a.options.ReasoningEffort = "", "", ""
			a.setActiveSession(updated)
			a.syncOptions("model · " + updated.Provider + "/" + updated.Model)
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
	a.models.show(nil)
	a.refreshModelPicker()
}

func (a *app) dismissModelPicker() {
	a.models.dismiss(func() { a.operations.Cancel(pickerCatalogOperation) })
}

func (a *app) refreshModelPicker() {
	presentation := a.models.dialog.Identity()
	if presentation == nil {
		return
	}
	a.tasks.session(pickerCatalogOperation, true,
		func(ctx context.Context) ([]protocol.Model, error) { return a.runtime.ListModels(ctx) },
		func(models []protocol.Model, err error) {
			if !presentation.current(a.models.dialog.Identity()) {
				return
			}
			if err != nil && len(models) == 0 {
				a.message("could not load models: " + err.Error())
				return
			}
			a.models.picker.SetItems(models)
			a.status.note("choose a provider-qualified model")
			if err != nil {
				a.message("model catalog incomplete: " + err.Error())
			}
		},
	)
}

func (a *app) ShowRuntimeStatus() {
	a.tasks.session(pickerCatalogOperation, true,
		func(ctx context.Context) (protocol.ApprovalModeResult, error) { return a.runtime.GetApprovalMode(ctx) },
		func(result protocol.ApprovalModeResult, err error) {
			if err != nil {
				a.message("could not read runtime status: " + err.Error())
				return
			}
			a.transcript.Append(&kit.Entry{
				Theme: a.transcript.theme, Label: "runtime options",
				Body: runtimeStatusText(a.runtimeProfile, a.displayOptions(), result.Mode),
			})
		},
	)
}

func runtimeStatusText(profile RuntimeProfile, options authoring.RunOptions, mode protocol.ApprovalMode) string {
	lines := []string{
		"model: " + modelLabel(options),
		"approval mode: " + string(mode),
	}
	features := profile.AvailableFeatureNames()
	if len(features) == 0 {
		features = []string{"none"}
	}
	discovery := profile.Discovery()
	capabilities := discovery.Capabilities
	limits := capabilities.Limits
	profileLines := []string{
		fmt.Sprintf("runtime: %s %s", discovery.ServerInfo.Name, discovery.ServerInfo.Version),
		"protocol: " + discovery.ProtocolVersion,
		"default workspace: " + discovery.ServerInfo.DefaultWorkspace.Path,
		"home: " + discovery.ServerInfo.Home,
		"available features: " + strings.Join(features, ", "),
		fmt.Sprintf("run concurrency: %s", runConcurrencyLabel(limits.MaxConcurrentRuns)),
		fmt.Sprintf("run replay: %d events / %s / %s", limits.RunReplay.MaxEvents, formatRuntimeBytes(limits.RunReplay.MaxBytes), limits.RunReplay.Scope),
		"command replay retention: " + formatRuntimeSeconds(limits.Idempotency.RetentionSeconds),
		"MCP authorization retention: " + formatRuntimeSeconds(limits.MCPAuthorizationAttempts.RetentionSeconds),
		fmt.Sprintf("runtime subscriptions: %d topics / %d watches", limits.RuntimeSubscription.MaxTopics, limits.RuntimeSubscription.MaxWatches),
		fmt.Sprintf("surface: %d run events / %d topics / %d streaming methods", len(capabilities.RunEvents), len(capabilities.RuntimeTopics), len(capabilities.StreamingMethods)),
	}
	return strings.Join(slices.Concat(profileLines, lines), "\n")
}

func runConcurrencyLabel(maximum *int) string {
	if maximum == nil {
		return "unbounded"
	}
	return fmt.Sprintf("at most %d runs", *maximum)
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

func approvalRulesDocument(rules []protocol.ApprovalRule) (readerDocument, error) {
	if len(rules) == 0 {
		return paragraphDocument("Approval rules", "none remembered", []string{"No remembered approval rules."}), nil
	}
	lines := make([]string, 0, len(rules))
	for _, rule := range rules {
		subject := conversation.ApprovalSubject(rule.Subject)
		source, err := mcp.ToolLabel(rule.Tool)
		if err != nil {
			return readerDocument{}, err
		}
		lines = append(lines, fmt.Sprintf("%s  %s  %s  %s:%s  %s  stale=%t", rule.ID, rule.Scope, rule.Decision, rule.ModelName, subject, source, rule.Stale))
	}
	return paragraphDocument("Approval rules", fmt.Sprintf("%d remembered", len(rules)), lines), nil
}

func resolveApprovalRule(rules []protocol.ApprovalRule, identity string) (protocol.ApprovalRule, error) {
	return resolveByID(rules, identity, "approval rule", func(rule protocol.ApprovalRule) string { return rule.ID })
}

type approvalRuleDeletionResult struct {
	id    string
	rules []protocol.ApprovalRule
}

func (a *app) syncOptions(message string) {
	a.prompt.SetOptions(a.displayOptions())
	a.brand.SetOptions(a.displayOptions())
	a.message(message)
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

// approvalPolicyDetail describes a mode from the gates the Runtime publishes
// for it, so the picker cannot drift from the policy it selects.
func approvalPolicyDetail(policy protocol.ApprovalModePolicy) string {
	return "write " + approvalGateText(policy.Write) +
		" · exec " + approvalGateText(policy.Exec) +
		" · network " + approvalGateText(policy.Network)
}

func approvalGateText(gate protocol.ApprovalGate) string {
	switch gate {
	case protocol.ApprovalGatePass:
		return "allowed"
	case protocol.ApprovalGatePrompt:
		return "asks first"
	case protocol.ApprovalGateDeny:
		return "refused"
	default:
		return string(gate)
	}
}
