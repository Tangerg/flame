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

// runOptionScreens chooses the presented Session's model and shows the model
// catalog and the Runtime's options. A chosen model becomes the Session's
// own choice through the Session lifecycle.
type runOptionScreens struct {
	screenHost
	service   Runtime
	profile   RuntimeProfile
	options   *authoring.RunOptions
	lifecycle *sessionLifecycle
	models    *pickerDialog[protocol.Model]

	observing      func() bool
	displayOptions func() authoring.RunOptions
	presentOptions func()
	appendNote     func(label, body string)
}

func (o *runOptionScreens) buildModelPicker(theme kit.Theme, glyphs kit.Glyphs) {
	o.models = newPickerDialog(pickerDialogConfig[protocol.Model]{
		stack: o.stack, theme: theme, glyphs: glyphs, title: "Models", search: "search models",
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
		choose:    o.selectSessionModel,
		dismissed: func() { o.tasks.operations.Cancel(pickerCatalogOperation) },
	})
}

func (o *runOptionScreens) selectSessionModel(model protocol.Model) {
	sessionID := o.lifecycle.state.current.ID
	o.lifecycle.runSessionChange("selecting model",
		func(ctx context.Context) (conversation.Session, error) {
			latest, err := o.service.GetSession(ctx, sessionID)
			if err != nil {
				return conversation.Session{}, err
			}
			return o.service.UpdateSession(ctx, conversation.UpdateSession{
				SessionID:        sessionID,
				Model:            &conversation.ModelRef{Provider: model.Provider, Model: model.ID},
				ExpectedRevision: latest.Session.Revision,
			})
		},
		func(updated conversation.Session) error {
			// The Session now owns the choice. A retained launch override would
			// outrank it here and follow the user into every other Session.
			o.options.Provider, o.options.Model, o.options.ReasoningEffort = "", "", ""
			o.lifecycle.setActiveSession(updated)
			o.syncOptions("model · " + updated.Provider + "/" + updated.Model)
			return nil
		},
	)
}

func (o *runOptionScreens) ChooseModel() {
	if o.observing() {
		o.message("model changes apply between runs")
		return
	}
	o.message("loading models")
	o.models.show(nil)
	o.refreshModelPicker()
}

func (o *runOptionScreens) dismissModelPicker() {
	o.models.dismiss(func() { o.tasks.operations.Cancel(pickerCatalogOperation) })
}

func (o *runOptionScreens) refreshModelPicker() {
	presentation := o.models.dialog.Identity()
	if presentation == nil {
		return
	}
	o.tasks.session(pickerCatalogOperation, true,
		func(ctx context.Context) ([]protocol.Model, error) { return o.service.ListModels(ctx) },
		func(models []protocol.Model, err error) {
			if !presentation.current(o.models.dialog.Identity()) {
				return
			}
			if err != nil && len(models) == 0 {
				o.message("could not load models: " + err.Error())
				return
			}
			o.models.picker.SetItems(models)
			o.status.note("choose a provider-qualified model")
			if err != nil {
				o.message("model catalog incomplete: " + err.Error())
			}
		},
	)
}

func (o *runOptionScreens) ShowRuntimeStatus() {
	o.tasks.session(pickerCatalogOperation, true,
		func(ctx context.Context) (protocol.ApprovalModeResult, error) { return o.service.GetApprovalMode(ctx) },
		func(result protocol.ApprovalModeResult, err error) {
			if err != nil {
				o.message("could not read runtime status: " + err.Error())
				return
			}
			o.appendNote("runtime options", runtimeStatusText(o.profile, o.displayOptions(), result.Mode))
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

func (o *runOptionScreens) syncOptions(message string) {
	o.presentOptions()
	o.message(message)
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
