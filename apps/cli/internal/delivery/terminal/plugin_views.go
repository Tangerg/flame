package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/extensions"
	"github.com/Tangerg/flame/apps/cli/internal/application/pluginview"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/packages/go-sdk/timezone"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// maxPluginViewPages bounds one list read; a view is a management surface, not
// an export.
const maxPluginViewPages = 50

// pluginViewState holds the terminal views installed packages declare. Each
// view becomes a slash command; the open list remembers its form so a data
// change re-reads the same rows.
type pluginViewState struct {
	views  []pluginview.View
	open   string
	form   map[string]string
	dialog *kit.Dialog
}

func (a *app) loadPluginViews() {
	if a.plugins == nil || !a.runtimeSupports(protocol.FeaturePlugins) {
		return
	}
	type loaded struct {
		views    []pluginview.View
		problems []error
	}
	a.tasks.application(pluginViewsOperation, true,
		func(ctx context.Context) (loaded, error) {
			installations, err := a.plugins.ListPlugins(ctx)
			if err != nil {
				return loaded{}, err
			}
			views, problems := pluginview.Views(installations)
			return loaded{views: views, problems: problems}, nil
		},
		func(result loaded, err error) {
			if err != nil {
				a.message("plugin views: " + err.Error())
				return
			}
			for _, problem := range result.problems {
				a.message(problem.Error())
			}
			a.pluginViews.views = result.views
			a.registerCommands()
		},
	)
}

// refreshPluginViews re-reads the declared views when installations change.
func (a *app) refreshPluginViews(affected bool) {
	if affected {
		a.loadPluginViews()
	}
}

func (a *app) registerPluginViewCommands() {
	for _, view := range a.pluginViews.views {
		descriptor := extensions.CommandDescriptor{Name: view.ID, Title: view.Title + " · " + pluginViewUsage(view), Category: commandCategoryExtensions, Arguments: extensions.OptionalArguments}
		if err := a.commands.add(view.InstallationID, descriptor, func(argument string, accept func()) {
			accept()
			if err := a.runPluginView(view, argument); err != nil {
				a.message(err.Error())
			}
		}, nil); err != nil {
			a.message(err.Error())
		}
	}
}

func pluginViewUsage(view pluginview.View) string {
	actions := make([]string, 0, len(view.Actions))
	for _, action := range view.Actions {
		usage := action.ID
		if action.Target == extension.TerminalActionRow {
			usage += " <" + strings.ToLower(view.List.Key) + ">"
		}
		actions = append(actions, usage)
	}
	return strings.Join(actions, " · ")
}

func (a *app) pluginViewContext() pluginview.Context {
	zone, _ := timezone.Host()
	return pluginview.Context{Workspace: a.session.current.Workspace.Path, TimeZone: zone}
}

// runPluginView lists the view, or runs the action the argument names first.
// Remaining words fill the list's form fields in order, or name the row.
func (a *app) runPluginView(view pluginview.View, argument string) error {
	words := strings.Fields(argument)
	if len(words) > 0 {
		if action, found := view.Action(words[0]); found {
			return a.startPluginViewAction(view, action, strings.Join(words[1:], " "))
		}
	}
	fields, err := view.ListForm(a.pluginViewContext())
	if err != nil {
		return err
	}
	if len(words) > len(fields) {
		return fmt.Errorf("/%s %s", view.ID, pluginViewUsage(view))
	}
	form := make(map[string]string, len(fields))
	for index, field := range fields {
		form[field.Path] = field.Initial
		if index < len(words) {
			form[field.Path] = words[index]
		}
	}
	a.showPluginView(view, form)
	return nil
}

func (a *app) showPluginView(view pluginview.View, form map[string]string) {
	a.pluginViews.open, a.pluginViews.form = view.ID, form
	a.reader.show(a.pluginViewReaderQuery(view, form))
}

func (a *app) pluginViewReaderQuery(view pluginview.View, form map[string]string) *readerQuery {
	here := a.pluginViewContext()
	return &readerQuery{
		status: "loading " + strings.ToLower(view.Title), follows: followsTopic(protocol.TopicPluginDataChanged),
		read: func(ctx context.Context) (readerDocument, error) {
			rows, err := a.readPluginViewRows(ctx, view, here, form)
			if err != nil {
				return readerDocument{}, err
			}
			return pluginViewDocument(view, rows), nil
		},
	}
}

func (a *app) readPluginViewRows(ctx context.Context, view pluginview.View, here pluginview.Context, form map[string]string) ([]pluginview.Row, error) {
	var rows []pluginview.Row
	cursor := ""
	for range maxPluginViewPages {
		request, err := view.ListRequest(here, form, cursor)
		if err != nil {
			return nil, err
		}
		response, err := a.plugins.CallPlugin(ctx, request, "")
		if err != nil {
			return nil, err
		}
		page, err := view.Page(response)
		if err != nil {
			return nil, err
		}
		rows = append(rows, page.Rows...)
		if page.Next == "" || view.List.Cursor == nil {
			return rows, nil
		}
		cursor = page.Next
	}
	return rows, nil
}

func pluginViewDocument(view pluginview.View, rows []pluginview.Row) readerDocument {
	detail := fmt.Sprintf("%d rows · /%s %s", len(rows), view.ID, pluginViewUsage(view))
	if len(rows) == 0 {
		return paragraphDocument(view.Title, detail, []string{"Nothing to show."})
	}
	width := 0
	for _, column := range view.List.Columns {
		width = max(width, len(column.Title))
	}
	sections := make([]toolSection, 0, len(rows))
	for _, row := range rows {
		lines := make([]string, 0, len(view.List.Columns))
		for _, column := range view.List.Columns {
			lines = append(lines, fmt.Sprintf("%-*s  %s", width, column.Title, row.Text(column.Field)))
		}
		sections = append(sections, toolSection{Title: row.Text(view.List.Key), Style: toolSectionCode, Text: strings.Join(lines, "\n")})
	}
	return readerDocument{Title: view.Title, Detail: detail, Sections: sections}
}

// startPluginViewAction resolves the selected row, asks for the action's form
// fields, confirms when declared, and then runs it.
func (a *app) startPluginViewAction(view pluginview.View, action protocol.PluginTerminalAction, key string) error {
	key = strings.TrimSpace(key)
	if action.Target == extension.TerminalActionView {
		return a.preparePluginViewAction(view, action, nil, "")
	}
	if key == "" {
		return fmt.Errorf("/%s %s needs a %s", view.ID, action.ID, strings.ToLower(view.List.Key))
	}
	if a.workbench != nil {
		if _, pending := a.workbench.PendingInstalledCommand(pluginViewIntent(view, action, key)); pending {
			a.runPluginViewAction(view, action, nil, key, nil)
			return nil
		}
	}
	form := a.pluginViews.form
	if a.pluginViews.open != view.ID {
		fields, err := view.ListForm(a.pluginViewContext())
		if err != nil {
			return err
		}
		form = make(map[string]string, len(fields))
		for _, field := range fields {
			form[field.Path] = field.Initial
		}
	}
	here := a.pluginViewContext()
	a.tasks.application(pluginViewOperation, false,
		func(ctx context.Context) (pluginview.Row, error) {
			rows, err := a.readPluginViewRows(ctx, view, here, form)
			if err != nil {
				return nil, err
			}
			return selectPluginViewRow(view, rows, key)
		},
		func(row pluginview.Row, err error) {
			if err != nil {
				a.message(err.Error())
				return
			}
			if err := a.preparePluginViewAction(view, action, row, row.Text(view.List.Key)); err != nil {
				a.message(err.Error())
			}
		},
	)
	return nil
}

// selectPluginViewRow finds the row whose key is key, or the one row whose key
// begins with it.
func selectPluginViewRow(view pluginview.View, rows []pluginview.Row, key string) (pluginview.Row, error) {
	var prefixed []pluginview.Row
	for _, row := range rows {
		value := row.Text(view.List.Key)
		if value == key {
			return row, nil
		}
		if strings.HasPrefix(value, key) {
			prefixed = append(prefixed, row)
		}
	}
	switch len(prefixed) {
	case 1:
		return prefixed[0], nil
	case 0:
		return nil, fmt.Errorf("%s has no %s %s", view.Title, strings.ToLower(view.List.Key), key)
	default:
		return nil, fmt.Errorf("%s %s matches %d rows", strings.ToLower(view.List.Key), key, len(prefixed))
	}
}

func (a *app) preparePluginViewAction(view pluginview.View, action protocol.PluginTerminalAction, row pluginview.Row, key string) error {
	fields, err := view.ActionForm(action, a.pluginViewContext(), row)
	if err != nil {
		return err
	}
	run := func(form map[string]string) {
		if !action.Confirm {
			a.runPluginViewAction(view, action, row, key, form)
			return
		}
		subject := view.Title
		if key != "" {
			subject += " " + key
		}
		a.confirmAction(action.Title, action.Title+" "+subject+"?", action.Title, func() {
			a.runPluginViewAction(view, action, row, key, form)
		})
	}
	if len(fields) == 0 {
		run(nil)
		return nil
	}
	a.openPluginViewForm(view, action, key, fields, run)
	return nil
}

func pluginViewIntent(view pluginview.View, action protocol.PluginTerminalAction, key string) string {
	return "plugin-view:" + view.InstallationID + "/" + view.ID + "/" + action.ID + "/" + key
}

// runPluginViewAction calls the action's method. A mutation is staged durably
// first, so a lost acknowledgement resumes the original call; row is nil when
// that staged call is being reconciled.
func (a *app) runPluginViewAction(view pluginview.View, action protocol.PluginTerminalAction, row pluginview.Row, key string, form map[string]string) {
	here := a.pluginViewContext()
	intent := pluginViewIntent(view, action, key)
	prepare := func(context.Context) (protocol.CallPluginRequest, error) {
		if action.Target == extension.TerminalActionRow && row == nil {
			return protocol.CallPluginRequest{}, errors.New("the selected row is no longer known")
		}
		return view.ActionRequest(action, here, row, form)
	}
	a.status.note(strings.ToLower(action.Title) + " · " + view.Title)
	started := a.tasks.application(pluginViewOperation, false,
		func(ctx context.Context) (struct{}, error) {
			if view.Mutates(action) {
				return struct{}{}, workbench.ExecuteInstalledCommand(ctx, a.plugins, a.workbench, intent, a.replayPolicy, prepare)
			}
			request, err := prepare(ctx)
			if err != nil {
				return struct{}{}, err
			}
			response, err := a.plugins.CallPlugin(ctx, request, "")
			if err == nil && response != nil && response.Failure != nil {
				err = errors.New(response.Failure.Message)
			}
			return struct{}{}, err
		},
		func(_ struct{}, err error) {
			if err != nil {
				message := action.Title + " failed: " + err.Error()
				if a.workbench != nil {
					if _, pending := a.workbench.PendingInstalledCommand(intent); pending {
						message += " · retry /" + view.ID + " " + action.ID + " " + key + " to reconcile the original call"
					}
				}
				a.message(message)
				return
			}
			a.message(view.Title + " · " + strings.ToLower(action.Title) + " done")
			a.reader.reload()
		},
	)
	if !started {
		a.message("another plugin view action is running")
	}
}

func (a *app) openPluginViewForm(view pluginview.View, action protocol.PluginTerminalAction, key string, fields []pluginview.FormField, submit func(map[string]string)) {
	if a.pluginViews.dialog != nil {
		a.pluginViews.dialog.Controller().Dismiss()
		a.pluginViews.dialog = nil
	}
	generation := a.session.context
	values := make(map[string]*string, len(fields))
	controls := make([]headless.Field, 0, len(fields))
	for _, field := range fields {
		value := field.Initial
		values[field.Path] = &value
		switch field.Kind {
		case extension.TerminalFormChoice, extension.TerminalFormBoolean:
			options := field.Options
			if field.Kind == extension.TerminalFormBoolean {
				options = []string{"true", "false"}
			}
			choice := headless.NewSelect(headless.SelectConfig[string]{Same: headless.Equal[string], Label: field.Title, Value: headless.Bind(values[field.Path]), Rows: min(len(options), 6)})
			entries := make([]headless.Option[string], 0, len(options))
			for _, option := range options {
				entries = append(entries, headless.Option[string]{Label: option, Value: option})
			}
			choice.SetOptions(entries)
			controls = append(controls, choice)
		default:
			text := headless.NewText(headless.TextConfig{Label: field.Title, Value: headless.Bind(values[field.Path])})
			text.Clipboard = a.loop.Clipboard()
			controls = append(controls, text)
		}
	}
	form := headless.NewForm(controls...)
	form.Keys = headless.DefaultFormKeys()
	var dialog *kit.Dialog
	dismiss := func() {
		if a.pluginViews.dialog == dialog {
			dialog.Controller().Dismiss()
			a.pluginViews.dialog = nil
		}
	}
	form.Done = func() {
		if a.pluginViews.dialog != dialog || !a.session.context.current(generation) {
			return
		}
		entered := make(map[string]string, len(values))
		for path, value := range values {
			entered[path] = strings.TrimSpace(*value)
		}
		dismiss()
		submit(entered)
	}
	form.GaveUp = dismiss
	body := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	title := action.Title + " · " + view.Title
	if key != "" {
		title += " · " + key
	}
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: title, Body: body,
		Where: layout.Placement{Width: 88, Height: formDialogHeight(body.HeightForWidth(84), len(controls), 24)},
	})
	a.pluginViews.dialog = dialog
	dialog.Controller().Show()
}
