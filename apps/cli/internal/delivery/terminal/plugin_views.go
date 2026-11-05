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

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

// maxPluginViewPages bounds one list read; a view is a management surface, not
// an export.
const maxPluginViewPages = 50

// pluginScreens shows installed plugins and the terminal views their releases
// declare. Each view becomes a slash command; open and form name the listed
// view and its form, so an action finds its row among the rows shown.
type pluginScreens struct {
	screenHost
	service   Plugins
	workbench *workbench.Store
	// viewsChanged rebuilds the command catalog after the views change.
	viewsChanged func()
	views        []pluginview.View
	open         string
	form         map[string]string
	dialog       *presentationDialog
}

func (p *pluginScreens) loadPluginViews() {
	type loaded struct {
		views    []pluginview.View
		problems []error
	}
	p.tasks.application(pluginViewsOperation, true,
		func(ctx context.Context) (loaded, error) {
			installations, err := p.service.ListPlugins(ctx)
			if err != nil {
				return loaded{}, err
			}
			views, problems := pluginview.Views(installations)
			return loaded{views: views, problems: problems}, nil
		},
		func(result loaded, err error) {
			if err != nil {
				p.message("plugin views: " + err.Error())
				return
			}
			for _, problem := range result.problems {
				p.message(problem.Error())
			}
			p.views = result.views
			p.viewsChanged()
		},
	)
}

func viewCommandName(view pluginview.View) string {
	return extensions.InstalledViewCommandPrefix + view.InstallationID + ":" + view.ID
}

func (p *pluginScreens) registerViewCommands(catalog *commandCatalog) {
	names := make(map[string]int)
	for _, view := range p.views {
		names[view.ID]++
	}
	for _, view := range p.views {
		descriptor := extensions.CommandDescriptor{Name: viewCommandName(view), Title: view.Title + " · " + pluginViewUsage(view), Category: commandCategoryExtensions, Arguments: extensions.OptionalArguments}
		if _, taken := catalog.lookup(view.ID); !taken && names[view.ID] == 1 {
			descriptor.Aliases = []string{view.ID}
		}
		if err := catalog.add(view.InstallationID, descriptor, func(argument string, accept func()) {
			accept()
			if err := p.runPluginView(view, argument); err != nil {
				p.message(err.Error())
			}
		}, nil); err != nil {
			p.message(err.Error())
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

func (p *pluginScreens) pluginViewContext() pluginview.Context {
	zone, _ := timezone.Host()
	return pluginview.Context{Workspace: p.session.current.Workspace.Path, TimeZone: zone}
}

// runPluginView lists the view, or runs the action the argument names first.
// Remaining words fill the list's form fields in order, or name the row.
func (p *pluginScreens) runPluginView(view pluginview.View, argument string) error {
	words := strings.Fields(argument)
	if len(words) > 0 {
		if action, found := view.Action(words[0]); found {
			return p.startPluginViewAction(view, action, strings.Join(words[1:], " "))
		}
	}
	fields, err := view.ListForm(p.pluginViewContext())
	if err != nil {
		return err
	}
	if len(words) > len(fields) {
		return fmt.Errorf("/%s %s", viewCommandName(view), pluginViewUsage(view))
	}
	form := make(map[string]string, len(fields))
	for index, field := range fields {
		form[field.Path] = field.Initial
		if index < len(words) {
			form[field.Path] = words[index]
		}
	}
	p.showPluginView(view, form)
	return nil
}

func (p *pluginScreens) showPluginView(view pluginview.View, form map[string]string) {
	p.open, p.form = viewCommandName(view), form
	p.reader.show(p.pluginViewReaderQuery(view, form))
}

func (p *pluginScreens) pluginViewReaderQuery(view pluginview.View, form map[string]string) *readerQuery {
	here := p.pluginViewContext()
	return &readerQuery{
		status: "loading " + strings.ToLower(view.Title), follows: followsTopic(protocol.TopicPluginDataChanged),
		read: func(ctx context.Context) (readerDocument, error) {
			rows, err := p.readPluginViewRows(ctx, view, here, form)
			if err != nil {
				return readerDocument{}, err
			}
			return pluginViewDocument(view, rows), nil
		},
	}
}

func (p *pluginScreens) readPluginViewRows(ctx context.Context, view pluginview.View, here pluginview.Context, form map[string]string) ([]pluginview.Row, error) {
	var rows []pluginview.Row
	seen := make(map[string]bool)
	cursor := ""
	for range maxPluginViewPages {
		request, err := view.ListRequest(here, form, cursor)
		if err != nil {
			return nil, err
		}
		response, err := p.service.CallPlugin(ctx, request, "")
		if err != nil {
			return nil, err
		}
		page, err := view.Page(response)
		if err != nil {
			return nil, err
		}
		rows = append(rows, page.Rows...)
		if page.Next == "" || view.List.Paging == nil {
			return rows, nil
		}
		if seen[page.Next] {
			return nil, errors.New("plugin view pagination repeated a cursor")
		}
		seen[page.Next] = true
		cursor = page.Next
	}
	return nil, fmt.Errorf("plugin view exceeds the %d-page limit; narrow the query", maxPluginViewPages)
}

func pluginViewDocument(view pluginview.View, rows []pluginview.Row) readerDocument {
	detail := fmt.Sprintf("%d rows · /%s %s", len(rows), viewCommandName(view), pluginViewUsage(view))
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
func (p *pluginScreens) startPluginViewAction(view pluginview.View, action protocol.PluginTerminalAction, key string) error {
	key = strings.TrimSpace(key)
	if action.Target == extension.TerminalActionView {
		return p.preparePluginViewAction(view, action, nil, "")
	}
	if key == "" {
		return fmt.Errorf("/%s %s needs a %s", viewCommandName(view), action.ID, strings.ToLower(view.List.Key))
	}
	if p.workbench != nil {
		if _, pending := p.workbench.PendingInstalledCommand(pluginViewIntent(view, action, key)); pending {
			p.runPluginViewAction(view, action, nil, key, nil)
			return nil
		}
	}
	form := p.form
	if p.open != viewCommandName(view) {
		fields, err := view.ListForm(p.pluginViewContext())
		if err != nil {
			return err
		}
		form = make(map[string]string, len(fields))
		for _, field := range fields {
			form[field.Path] = field.Initial
		}
	}
	here := p.pluginViewContext()
	p.tasks.application(pluginViewOperation, false,
		func(ctx context.Context) (pluginview.Row, error) {
			rows, err := p.readPluginViewRows(ctx, view, here, form)
			if err != nil {
				return nil, err
			}
			return selectPluginViewRow(view, rows, key)
		},
		func(row pluginview.Row, err error) {
			if err != nil {
				p.message(err.Error())
				return
			}
			if err := p.preparePluginViewAction(view, action, row, row.Text(view.List.Key)); err != nil {
				p.message(err.Error())
			}
		},
	)
	return nil
}

// selectPluginViewRow finds the row whose key is key, or the one row whose key
// begins with it.
func selectPluginViewRow(view pluginview.View, rows []pluginview.Row, key string) (pluginview.Row, error) {
	var exact, prefixed []pluginview.Row
	for _, row := range rows {
		value := row.Text(view.List.Key)
		if value == key {
			exact = append(exact, row)
		} else if strings.HasPrefix(value, key) {
			prefixed = append(prefixed, row)
		}
	}
	if len(exact) > 0 {
		prefixed = exact
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

func (p *pluginScreens) preparePluginViewAction(view pluginview.View, action protocol.PluginTerminalAction, row pluginview.Row, key string) error {
	fields, err := view.ActionForm(action, p.pluginViewContext(), row)
	if err != nil {
		return err
	}
	run := func(form map[string]string) {
		if !action.Confirm {
			p.runPluginViewAction(view, action, row, key, form)
			return
		}
		subject := view.Title
		if key != "" {
			subject += " " + key
		}
		p.confirm(action.Title, action.Title+" "+subject+"?", action.Title, "", func() {
			p.runPluginViewAction(view, action, row, key, form)
		})
	}
	if len(fields) == 0 {
		run(nil)
		return nil
	}
	p.openPluginViewForm(view, action, key, fields, run)
	return nil
}

func pluginViewIntent(view pluginview.View, action protocol.PluginTerminalAction, key string) string {
	return "plugin-view:" + view.InstallationID + "/" + view.ID + "/" + action.ID + "/" + key
}

// runPluginViewAction calls the action's method. A mutation is staged durably
// first, so a lost acknowledgement resumes the original call; row is nil when
// that staged call is being reconciled.
func (p *pluginScreens) runPluginViewAction(view pluginview.View, action protocol.PluginTerminalAction, row pluginview.Row, key string, form map[string]string) {
	here := p.pluginViewContext()
	intent := pluginViewIntent(view, action, key)
	prepare := func(context.Context) (protocol.CallPluginRequest, error) {
		if action.Target == extension.TerminalActionRow && row == nil {
			return protocol.CallPluginRequest{}, errors.New("the selected row is no longer known")
		}
		return view.ActionRequest(action, here, row, form)
	}
	p.status.note(strings.ToLower(action.Title) + " · " + view.Title)
	started := p.tasks.application(pluginViewOperation, false,
		func(ctx context.Context) (struct{}, error) {
			if view.Mutates(action) {
				return struct{}{}, workbench.ExecuteInstalledCommand(ctx, p.service, p.workbench, intent, p.replayPolicy, prepare)
			}
			request, err := prepare(ctx)
			if err != nil {
				return struct{}{}, err
			}
			response, err := p.service.CallPlugin(ctx, request, "")
			if err == nil && response != nil && response.Failure != nil {
				err = errors.New(response.Failure.Message)
			}
			return struct{}{}, err
		},
		func(_ struct{}, err error) {
			if err != nil {
				message := action.Title + " failed: " + err.Error()
				if p.workbench != nil {
					if _, pending := p.workbench.PendingInstalledCommand(intent); pending {
						message += " · retry /" + viewCommandName(view) + " " + action.ID + " " + key + " to reconcile the original call"
					}
				}
				p.message(message)
				return
			}
			p.message(view.Title + " · " + strings.ToLower(action.Title) + " done")
			p.reader.reload()
		},
	)
	if !started {
		p.message("another plugin view action is running")
	}
}

func (p *pluginScreens) openPluginViewForm(view pluginview.View, action protocol.PluginTerminalAction, key string, fields []pluginview.FormField, submit func(map[string]string)) {
	if p.dialog != nil {
		p.dialog.Dismiss()
		p.dialog = nil
	}
	generation := p.session.context
	values := make(map[string]func() string, len(fields))
	controls := make([]base.Field, 0, len(fields))
	for _, field := range fields {
		switch field.Kind {
		case extension.TerminalFormChoice, extension.TerminalFormBoolean:
			options := field.Options
			if field.Kind == extension.TerminalFormBoolean {
				options = []string{"true", "false"}
			}
			choice := base.NewSelect(base.SelectConfig[string]{Same: base.Equal[string], Label: field.Title, Initial: field.Initial, Rows: min(len(options), 6)})
			entries := make([]base.Option[string], 0, len(options))
			for _, option := range options {
				entries = append(entries, base.Option[string]{Label: option, Value: option})
			}
			choice.SetOptions(entries)
			choice.Choose(field.Initial)
			values[field.Path] = func() string {
				option, ok := choice.Chosen()
				if !ok {
					return ""
				}
				return option.Value
			}
			controls = append(controls, choice)
		default:
			text := base.NewText(base.TextConfig{Label: field.Title, Initial: field.Initial})
			text.Clipboard = p.clipboard
			values[field.Path] = text.Text
			controls = append(controls, text)
		}
	}
	form := kit.FormConfig{Fields: controls, Theme: p.theme, Glyphs: p.glyphs,
		Hints: []keymap.Action{base.Submit, base.Cancel}}
	form.Keys = base.DefaultFormKeys()
	var dialog *presentationDialog
	dismiss := func() {
		if p.dialog == dialog {
			dialog.Dismiss()
			p.dialog = nil
		}
	}
	form.Submit = func(base.SubmissionID) (scene.ActionState, error) {
		if p.dialog != dialog || !p.session.context.current(generation) {
			return scene.Unchanged, nil
		}
		entered := make(map[string]string, len(values))
		for path, value := range values {
			entered[path] = value()
		}
		dismiss()
		submit(entered)
		return scene.Applied, nil
	}

	body := kit.NewForm(form)
	title := action.Title + " · " + view.Title
	if key != "" {
		title += " · " + key
	}
	dialog = newPresentationDialog(p.stack, kit.DialogConfig{Theme: p.theme, Glyphs: p.glyphs,
		Title: title, Body: formNavigation(body.Element(), dismiss),
		Where: layout.Placement{Width: 88, Height: formDialogHeight(body.HeightForWidth(84), len(controls), 24)},
	})
	p.dialog = dialog
	dialog.Show()
}
