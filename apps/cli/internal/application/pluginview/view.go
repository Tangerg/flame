// Package pluginview turns a release's declared terminal views into plugin
// calls and readable rows. A view carries every input binding and form-field
// kind, so the CLI renders any package's view without its types or schemas.
package pluginview

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// View is one declared terminal view bound to the installation that serves it.
type View struct {
	protocol.PluginTerminalView
	InstallationID string
	Digest         string
	contract       string
	capability     *plugincontract.Capability
}

// Presentation admission includes the availability of the selected release.
func Views(installations []protocol.PluginInstallation) ([]View, []error) {
	var views []View
	var problems []error
	for _, installation := range installations {
		if installation.Presentation != protocol.PluginPresentationAdmitted {
			continue
		}
		release := installation.Selected
		for _, declared := range release.TerminalViews {
			index := slices.IndexFunc(release.Contributions, func(value protocol.PluginContribution) bool { return value.ID == declared.Contribution })
			if index < 0 {
				problems = append(problems, fmt.Errorf("plugin %s view %s has no contribution %s", release.Name, declared.ID, declared.Contribution))
				continue
			}
			contract := release.Contributions[index].Contract
			capability, err := plugincontract.ParseCapability([]byte(contract))
			if err != nil {
				problems = append(problems, fmt.Errorf("plugin %s view %s contract: %w", release.Name, declared.ID, err))
				continue
			}
			views = append(views, View{PluginTerminalView: declared, InstallationID: installation.ID, Digest: release.Digest, contract: contract, capability: capability})
		}
	}
	return views, problems
}

// Context is what the terminal knows about where it runs.
type Context struct {
	Workspace string
	TimeZone  string
}

// Row is one listed item, read by '/'-separated field path.
type Row map[string]jsontext.Value

func (r Row) Field(path string) (jsontext.Value, bool) {
	segments := strings.Split(path, "/")
	value, found := r[segments[0]]
	for _, segment := range segments[1:] {
		if !found {
			return nil, false
		}
		var object map[string]jsontext.Value
		if json.Unmarshal(value, &object) != nil {
			return nil, false
		}
		value, found = object[segment]
	}
	return value, found
}

// Text renders one field for reading; a string shows without quotes.
func (r Row) Text(path string) string {
	value, found := r.Field(path)
	if !found {
		return ""
	}
	var text string
	if json.Unmarshal(value, &text) == nil {
		return text
	}
	return string(value)
}

// Page is one list response.
type Page struct {
	Rows []Row
	Next string
}

// FormField is one value a person enters, with its starting text.
type FormField struct {
	Path    string
	Title   string
	Kind    extension.TerminalFormKind
	Options []string
	Initial string
}

// ListForm is the list's own form fields, such as a scope to read.
func (v View) ListForm(context Context) ([]FormField, error) {
	return formFields(v.List.Input, context, nil)
}

// ActionForm is what a person fills before action runs on row.
func (v View) ActionForm(action protocol.PluginTerminalAction, context Context, row Row) ([]FormField, error) {
	return formFields(action.Input, context, row)
}

func (v View) Action(id string) (protocol.PluginTerminalAction, bool) {
	index := slices.IndexFunc(v.Actions, func(action protocol.PluginTerminalAction) bool { return action.ID == id })
	if index < 0 {
		return protocol.PluginTerminalAction{}, false
	}
	return v.Actions[index], true
}

// Mutates reports whether action changes the package's data, so its call must
// be staged durably.
func (v View) Mutates(action protocol.PluginTerminalAction) bool {
	method, found := v.capability.Method(action.Method)
	return found && method.Kind() == portable.Command
}

// ListRequest reads one page; cursor is empty for the first.
func (v View) ListRequest(context Context, form map[string]string, cursor string) (protocol.CallPluginRequest, error) {
	input := slices.Clone(v.List.Input)
	if v.List.Paging != nil {
		literal, err := json.Marshal(cursor)
		if err != nil {
			return protocol.CallPluginRequest{}, err
		}
		input = append(input, protocol.PluginTerminalInput{Path: v.List.Paging.Input, Value: protocol.PluginTerminalBinding{Source: extension.TerminalBindingValue, Value: string(literal)}})
	}
	return v.request(v.List.Method, input, context, nil, form)
}

// ActionRequest calls action; row is nil for a view action.
func (v View) ActionRequest(action protocol.PluginTerminalAction, context Context, row Row, form map[string]string) (protocol.CallPluginRequest, error) {
	if (action.Target == extension.TerminalActionRow) != (row != nil) {
		return protocol.CallPluginRequest{}, errors.New("action target and selected row disagree")
	}
	return v.request(action.Method, action.Input, context, row, form)
}

func (v View) request(method string, bindings []protocol.PluginTerminalInput, context Context, row Row, form map[string]string) (protocol.CallPluginRequest, error) {
	input := make(map[string]any)
	for _, binding := range bindings {
		value, err := resolve(binding.Value, context, row, form[binding.Path])
		if err != nil {
			return protocol.CallPluginRequest{}, fmt.Errorf("%s: %w", binding.Path, err)
		}
		place(input, strings.Split(binding.Path, "/"), value)
	}
	encoded, err := json.Marshal(input, json.Deterministic(true))
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	compiled, found := v.capability.Method(method)
	if !found {
		return protocol.CallPluginRequest{}, fmt.Errorf("method %s is not in the contract", method)
	}
	parsed, err := framevalue.ParseJSON(encoded)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	if err := compiled.ValidateInput(parsed); err != nil {
		return protocol.CallPluginRequest{}, err
	}
	return protocol.CallPluginRequest{
		InstallationID: v.InstallationID, Digest: v.Digest, Contribution: v.Contribution, Contract: v.contract,
		Method: method, Input: string(encoded),
		Target: portable.Target{Scope: compiled.Descriptor().Target, Reference: v.InstallationID},
	}, nil
}

// Page decodes a list response's rows and next cursor.
func (v View) Page(response *protocol.CallPluginResponse) (Page, error) {
	if response == nil {
		return Page{}, errors.New("missing list response")
	}
	if failure := response.Failure; failure != nil {
		return Page{}, fmt.Errorf("plugin list refused (%s): %s", failure.Code, failure.Message)
	}
	var output Row
	if err := json.Unmarshal([]byte(response.Output), &output); err != nil {
		return Page{}, fmt.Errorf("list output: %w", err)
	}
	var page Page
	rows, found := output.Field(v.List.Rows)
	if !found || rows.Kind() != '[' {
		return Page{}, fmt.Errorf("list output %q must contain the row array", v.List.Rows)
	}
	if err := json.Unmarshal(rows, &page.Rows); err != nil {
		return Page{}, fmt.Errorf("list rows: %w", err)
	}
	if paging := v.List.Paging; paging != nil {
		if raw, found := output.Field(paging.Output); found {
			if raw.Kind() != '"' {
				return Page{}, fmt.Errorf("list cursor %q must be a string", paging.Output)
			}
			if err := json.Unmarshal(raw, &page.Next); err != nil {
				return Page{}, fmt.Errorf("list cursor: %w", err)
			}
		}
	}
	return page, nil
}

func formFields(bindings []protocol.PluginTerminalInput, context Context, row Row) ([]FormField, error) {
	var fields []FormField
	for _, binding := range bindings {
		form := binding.Value.Form
		if binding.Value.Source != extension.TerminalBindingForm || form == nil {
			continue
		}
		field := FormField{Path: binding.Path, Title: form.Title, Kind: form.Kind, Options: slices.Clone(form.Options)}
		if form.Default != nil {
			value, err := resolve(*form.Default, context, row, "")
			if err != nil {
				return nil, fmt.Errorf("%s default: %w", binding.Path, err)
			}
			field.Initial = initialText(value)
		}
		fields = append(fields, field)
	}
	return fields, nil
}

func initialText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case jsontext.Value:
		var text string
		if json.Unmarshal(typed, &text) == nil {
			return text
		}
		return string(typed)
	default:
		encoded, _ := json.Marshal(typed)
		return string(encoded)
	}
}

func resolve(binding protocol.PluginTerminalBinding, context Context, row Row, entered string) (any, error) {
	switch binding.Source {
	case extension.TerminalBindingValue:
		return jsontext.Value(binding.Value), nil
	case extension.TerminalBindingRow:
		value, found := row.Field(binding.Field)
		if !found {
			return nil, fmt.Errorf("row has no %s", binding.Field)
		}
		return value, nil
	case extension.TerminalBindingContext:
		switch binding.Context {
		case extension.TerminalContextWorkspace:
			return context.Workspace, nil
		case extension.TerminalContextTimeZone:
			return context.TimeZone, nil
		}
		return nil, fmt.Errorf("unknown context %q", binding.Context)
	case extension.TerminalBindingForm:
		if binding.Form == nil {
			return nil, errors.New("form binding has no field")
		}
		return formValue(*binding.Form, entered)
	default:
		return nil, fmt.Errorf("unknown binding source %q", binding.Source)
	}
}

func formValue(form protocol.PluginTerminalFormField, entered string) (any, error) {
	switch form.Kind {
	case extension.TerminalFormText:
		return entered, nil
	case extension.TerminalFormChoice:
		if !slices.Contains(form.Options, entered) {
			return nil, fmt.Errorf("%s must be one of %s", form.Title, strings.Join(form.Options, ", "))
		}
		return entered, nil
	case extension.TerminalFormBoolean:
		value, err := strconv.ParseBool(entered)
		if err != nil {
			return nil, fmt.Errorf("%s must be true or false", form.Title)
		}
		return value, nil
	case extension.TerminalFormInteger:
		value, err := strconv.ParseInt(strings.TrimSpace(entered), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s must be a whole number", form.Title)
		}
		return value, nil
	case extension.TerminalFormNumber:
		value, err := strconv.ParseFloat(strings.TrimSpace(entered), 64)
		if err != nil {
			return nil, fmt.Errorf("%s must be a number", form.Title)
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unknown form kind %q", form.Kind)
	}
}

func place(object map[string]any, segments []string, value any) {
	if len(segments) == 1 {
		object[segments[0]] = value
		return
	}
	child, ok := object[segments[0]].(map[string]any)
	if !ok {
		child = make(map[string]any)
		object[segments[0]] = child
	}
	place(child, segments[1:], value)
}
