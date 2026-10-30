package terminal

import (
	"errors"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/schedule"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/timezone"

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
	timeZone     string
	enabled      bool
}

func newScheduleFormDraft(mode scheduleFormMode, scheduled schedule.Snapshot, defaultWorkspace string) scheduleFormDraft {
	if mode == scheduleFormUpdate {
		return scheduleFormDraft{
			title: scheduled.Title, instructions: scheduled.Instructions, workspace: scheduled.CWD,
			provider: scheduled.ModelSelection.Provider, model: scheduled.ModelSelection.Model, cron: scheduled.Cron, timeZone: scheduled.TimeZone, enabled: !scheduled.NextRunAt.IsZero(),
		}
	}
	// An unnamed host zone leaves the required field empty for the person to fill.
	hostZone, _ := timezone.Host()
	return scheduleFormDraft{workspace: defaultWorkspace, cron: defaultScheduleCron, timeZone: hostZone, enabled: true}
}

func (s scheduleFormDraft) candidate() (schedule.Draft, error) {
	if err := validateScheduleModelPair(s.provider, s.model); err != nil {
		return schedule.Draft{}, err
	}
	request := schedule.Draft{Title: strings.TrimSpace(s.title), Instructions: strings.TrimSpace(s.instructions), CWD: strings.TrimSpace(s.workspace), Cron: strings.TrimSpace(s.cron), TimeZone: strings.TrimSpace(s.timeZone), Enabled: true}
	request.ModelSelection.Provider, request.ModelSelection.Model = s.provider, s.model
	if request.Instructions == "" || request.Cron == "" || request.TimeZone == "" {
		return schedule.Draft{}, errors.New("instructions, cron and time zone are required")
	}
	return request, nil
}

func (s scheduleFormDraft) patch(original schedule.Snapshot) (schedule.UpdateInput, bool, error) {
	request := schedule.UpdateInput{ID: original.ID, ExpectedRevision: original.Revision}
	title := strings.TrimSpace(s.title)
	if title != original.Title {
		request.Title = &title
	}
	instructions := strings.TrimSpace(s.instructions)
	if instructions != original.Instructions {
		if instructions == "" {
			return schedule.UpdateInput{}, false, errors.New("instructions are required")
		}
		request.Instructions = &instructions
	}
	workspace := strings.TrimSpace(s.workspace)
	if workspace != original.CWD {
		request.CWD = &workspace
	}
	provider, model := s.provider, s.model
	if provider != original.ModelSelection.Provider || model != original.ModelSelection.Model {
		if err := validateScheduleModelPair(provider, model); err != nil {
			return schedule.UpdateInput{}, false, err
		}
		request.ModelSelection.Provider, request.ModelSelection.Model = &provider, &model
	}
	cron := strings.TrimSpace(s.cron)
	if cron != original.Cron {
		if cron == "" {
			return schedule.UpdateInput{}, false, errors.New("cron is required")
		}
		request.Cron = &cron
	}
	timeZone := strings.TrimSpace(s.timeZone)
	if timeZone != original.TimeZone {
		if timeZone == "" {
			return schedule.UpdateInput{}, false, errors.New("time zone is required")
		}
		request.TimeZone = &timeZone
	}
	enabled := s.enabled
	if enabled != !original.NextRunAt.IsZero() {
		request.Enabled = &enabled
	}
	if !scheduleRequestHasChanges(request) {
		return request, false, nil
	}
	return request, true, nil
}

func scheduleRequestHasChanges(request schedule.UpdateInput) bool {
	return request.Title != nil || request.Instructions != nil || request.CWD != nil || request.ModelSelection.Provider != nil || request.ModelSelection.Model != nil || request.ModelSelection.ReasoningEffort != nil || request.Cron != nil || request.TimeZone != nil || request.Enabled != nil
}

func (a *app) openScheduleForm(mode scheduleFormMode, scheduled schedule.Snapshot) {
	if a.dialogs.scheduleDialog != nil {
		a.dialogs.scheduleDialog.Controller().Dismiss()
		a.dialogs.scheduleDialog = nil
	}
	generation := a.session.context
	draft := newScheduleFormDraft(mode, scheduled, a.session.current.Workspace.Path)
	textField := func(label, placeholder string, value *string, check func(string) error) *headless.Text {
		field := headless.NewText(headless.TextConfig{Label: label, Placeholder: placeholder, Value: headless.Bind(value), Check: check})
		field.Clipboard = a.loop.Clipboard()
		return field
	}
	fields := []headless.Field{
		textField("Title", "Optional name", &draft.title, nil),
		textField("Instructions", "Prompt sent when this schedule fires", &draft.instructions, requiredText),
		textField("Cron", defaultScheduleCron, &draft.cron, requiredText),
		textField("Time zone", "IANA name, e.g. Asia/Shanghai", &draft.timeZone, requiredText),
		textField("Workspace", "Empty uses the runtime default", &draft.workspace, nil),
		textField("Provider", "Optional; set together with model", &draft.provider, nil),
		textField("Model", "Optional; set together with provider", &draft.model, func(string) error {
			return validateScheduleModelPair(draft.provider, draft.model)
		}),
	}
	if mode == scheduleFormUpdate {
		enabled := headless.NewSelect(headless.SelectConfig[bool]{Same: headless.Equal[bool], Label: "Lifecycle", Value: headless.Bind(&draft.enabled), Rows: 2})
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
		Where: layout.Placement{Width: 88, Height: formDialogHeight(body.HeightForWidth(84), len(fields), 24)},
	})
	a.dialogs.scheduleDialog = dialog
	dialog.Controller().Show()
}

func validateScheduleModelPair(provider, model string) error {
	if err := flamemodel.ValidateSelection(provider, model, ""); errors.Is(err, flamemodel.ErrIncomplete) {
		return errors.New("provider and model must both be set or both be empty")
	} else if err != nil {
		return err
	}
	return nil
}
