package terminal

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

const defaultScheduleCron = "0 9 * * 1-5"

type scheduleFormMode uint8

const (
	scheduleFormCreate scheduleFormMode = iota + 1
	scheduleFormUpdate
)

type scheduleFormDraft struct {
	title        string
	instructions string
	workspace    string
	provider     string
	model        string
	cron         string
	enabled      bool
}

func newScheduleFormDraft(mode scheduleFormMode, scheduled protocol.Schedule, defaultWorkspace string) scheduleFormDraft {
	if mode == scheduleFormUpdate {
		return scheduleFormDraft{
			title: scheduled.Title, instructions: scheduled.Instructions, workspace: scheduleWorkspacePath(scheduled),
			provider: scheduled.Provider, model: scheduled.Model, cron: scheduled.Cron, enabled: scheduled.Enabled,
		}
	}
	return scheduleFormDraft{workspace: defaultWorkspace, cron: defaultScheduleCron, enabled: true}
}

func (s scheduleFormDraft) candidate() (protocol.CreateScheduleRequest, error) {
	request := protocol.CreateScheduleRequest{
		Title: strings.TrimSpace(s.title), Instructions: strings.TrimSpace(s.instructions),
		Provider: s.provider, Model: s.model, Cron: strings.TrimSpace(s.cron),
	}
	workspace := strings.TrimSpace(s.workspace)
	if workspace != "" {
		if !filepath.IsAbs(workspace) {
			return protocol.CreateScheduleRequest{}, errors.New("workspace path is not absolute")
		}
		request.Workspace = &protocol.WorkspaceRef{Path: workspace}
	}
	if err := validateScheduleModelPair(request.Provider, request.Model); err != nil {
		return protocol.CreateScheduleRequest{}, err
	}
	if err := protocol.ValidateWireTree(request); err != nil {
		return protocol.CreateScheduleRequest{}, err
	}
	return request, nil
}

func (s scheduleFormDraft) patch(original protocol.Schedule) (protocol.UpdateScheduleRequest, bool, error) {
	request := protocol.UpdateScheduleRequest{ID: original.ID, ExpectedRevision: original.Revision}
	title := strings.TrimSpace(s.title)
	if title != original.Title {
		request.Title = &title
	}
	instructions := strings.TrimSpace(s.instructions)
	if instructions != original.Instructions {
		request.Instructions = &instructions
	}
	workspace := strings.TrimSpace(s.workspace)
	if workspace != scheduleWorkspacePath(original) {
		if workspace == "" {
			request.WorkspaceMode = protocol.ScheduleWorkspaceDefault
		} else {
			if !filepath.IsAbs(workspace) {
				return protocol.UpdateScheduleRequest{}, false, errors.New("workspace path is not absolute")
			}
			request.Workspace = &protocol.WorkspaceRef{Path: workspace}
		}
	}
	provider, model := s.provider, s.model
	if provider != original.Provider || model != original.Model {
		if err := validateScheduleModelPair(provider, model); err != nil {
			return protocol.UpdateScheduleRequest{}, false, err
		}
		request.Provider, request.Model = &provider, &model
	}
	cron := strings.TrimSpace(s.cron)
	if cron != original.Cron {
		request.Cron = &cron
	}
	enabled := s.enabled
	if enabled != original.Enabled {
		request.Enabled = &enabled
	}
	if !scheduleRequestHasChanges(request) {
		return request, false, nil
	}
	if err := protocol.ValidateWireTree(request); err != nil {
		return protocol.UpdateScheduleRequest{}, false, err
	}
	return request, true, nil
}

func scheduleWorkspacePath(scheduled protocol.Schedule) string {
	if scheduled.Workspace == nil {
		return ""
	}
	return scheduled.Workspace.Path
}

func scheduleRequestHasChanges(request protocol.UpdateScheduleRequest) bool {
	return request.Title != nil || request.Instructions != nil || request.Workspace != nil ||
		request.WorkspaceMode != "" || request.Provider != nil || request.Model != nil ||
		request.ReasoningEffort != nil || request.Cron != nil || request.Enabled != nil
}

func (a *app) openScheduleForm(mode scheduleFormMode, scheduled protocol.Schedule) {
	if a.dialogs.scheduleDialog != nil {
		a.dialogs.scheduleDialog.Controller().Dismiss()
		a.dialogs.scheduleDialog = nil
	}
	generation := a.session.context
	draft := newScheduleFormDraft(mode, scheduled, a.session.current.Workspace.Path)
	textField := func(label, placeholder string, value *string, check func(string) error) *headless.Text {
		field := &headless.Text{Label: label, Placeholder: placeholder, Value: headless.Bind(value), Check: check}
		field.Editor().Clipboard = a.loop.Clipboard()
		return field
	}
	fields := []headless.Field{
		textField("Title", "Optional name", &draft.title, nil),
		textField("Instructions", "Prompt sent when this schedule fires", &draft.instructions, requiredText),
		textField("Cron", defaultScheduleCron, &draft.cron, validateCronShape),
		textField("Workspace", "Empty uses the runtime default", &draft.workspace, nil),
		textField("Provider", "Optional; set together with model", &draft.provider, nil),
		textField("Model", "Optional; set together with provider", &draft.model, func(string) error {
			return validateScheduleModelPair(draft.provider, draft.model)
		}),
	}
	if mode == scheduleFormUpdate {
		enabled := &headless.Select[bool]{Label: "Lifecycle", Value: headless.Bind(&draft.enabled), Rows: 2}
		enabled.SetOptions([]headless.Option[bool]{{Label: "Enabled", Value: true}, {Label: "Disabled", Value: false}})
		fields = append(fields, enabled)
	}
	form := headless.NewForm(fields...)
	form.Keys = headless.DefaultFormKeys()
	var dialog *kit.Dialog
	dismiss := func() {
		if a.dialogs.scheduleDialog == dialog {
			dialog.Controller().Dismiss()
			a.dialogs.scheduleDialog = nil
		}
	}
	form.Done = func() {
		if a.dialogs.scheduleDialog != dialog || !a.session.context.current(generation) {
			return
		}
		switch mode {
		case scheduleFormCreate:
			request, err := draft.candidate()
			if err != nil {
				a.message("schedule form: " + err.Error())
				return
			}
			dismiss()
			a.createSchedule(request)
		case scheduleFormUpdate:
			request, changed, err := draft.patch(scheduled)
			if err != nil {
				a.message("schedule form: " + err.Error())
				return
			}
			dismiss()
			if !changed {
				a.message("schedule configuration unchanged")
				return
			}
			a.updateSchedule(request, "updating schedule "+scheduled.ID)
		}
	}
	form.GaveUp = dismiss
	body := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	title := "Create scheduled run"
	if mode == scheduleFormUpdate {
		title = "Edit scheduled run · " + scheduled.ID
	}
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: title, Body: body,
		Where: layout.Placement{Width: 88, Height: formDialogHeight(body.Measure(84), len(fields), 24)},
	})
	a.dialogs.scheduleDialog = dialog
	dialog.Controller().Show()
}

func validateCronShape(value string) error {
	fields := strings.Fields(value)
	if len(fields) != 5 {
		return errors.New("cron must contain exactly five fields")
	}
	return nil
}

func validateScheduleModelPair(provider, model string) error {
	if err := protocol.ValidateModelSelection(provider, model, ""); errors.Is(err, protocol.ErrIncompleteModelSelection) {
		return errors.New("provider and model must both be set or both be empty")
	} else if err != nil {
		return err
	}
	return nil
}
