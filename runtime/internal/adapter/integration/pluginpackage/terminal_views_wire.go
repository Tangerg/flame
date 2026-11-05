package pluginpackage

import (
	"encoding/json/jsontext"
	"maps"
	"slices"

	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type wireTerminalView struct {
	ID           string                `json:"id"`
	Title        string                `json:"title"`
	Contribution plugin.ContributionID `json:"contribution"`
	List         wireTerminalList      `json:"list"`
	Actions      []wireTerminalAction  `json:"actions,omitempty"`
}

type wireTerminalList struct {
	Method  string                 `json:"method"`
	Input   map[string]wireBinding `json:"input,omitempty"`
	Rows    string                 `json:"rows"`
	Key     string                 `json:"key"`
	Paging  *wireTerminalPaging    `json:"paging,omitzero"`
	Columns []wireTerminalColumn   `json:"columns"`
}

type wireTerminalPaging struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

type wireTerminalColumn struct {
	Title string `json:"title"`
	Field string `json:"field"`
}

type wireTerminalAction struct {
	ID      string                         `json:"id"`
	Title   string                         `json:"title"`
	Method  string                         `json:"method"`
	Target  extension.TerminalActionTarget `json:"target"`
	Confirm bool                           `json:"confirm,omitzero"`
	Input   map[string]wireBinding         `json:"input,omitempty"`
}

// wireBinding names exactly one source of an input value.
type wireBinding struct {
	Value   jsontext.Value             `json:"value,omitzero"`
	Row     *string                    `json:"row,omitzero"`
	Context *extension.TerminalContext `json:"context,omitzero"`
	Form    *wireFormField             `json:"form,omitzero"`
}

type wireFormField struct {
	Title   string       `json:"title"`
	Default *wireBinding `json:"default,omitzero"`
}

func (w wireTerminalView) domain() (plugin.TerminalView, error) {
	listInput, err := domainBindings(w.List.Input)
	if err != nil {
		return plugin.TerminalView{}, err
	}
	view := plugin.TerminalView{
		ID: w.ID, Title: w.Title, Contribution: w.Contribution,
		List: plugin.TerminalList{Method: w.List.Method, Input: listInput, Rows: w.List.Rows, Key: w.List.Key},
	}
	if w.List.Paging != nil {
		view.List.Paging = &plugin.TerminalPaging{Input: w.List.Paging.Input, Output: w.List.Paging.Output}
	}
	for _, column := range w.List.Columns {
		view.List.Columns = append(view.List.Columns, plugin.TerminalColumn{Title: column.Title, Field: column.Field})
	}
	for _, action := range w.Actions {
		input, err := domainBindings(action.Input)
		if err != nil {
			return plugin.TerminalView{}, err
		}
		view.Actions = append(view.Actions, plugin.TerminalAction{ID: action.ID, Title: action.Title, Method: action.Method, Target: action.Target, Confirm: action.Confirm, Input: input})
	}
	return view, nil
}

func domainBindings(bindings map[string]wireBinding) ([]plugin.InputBinding, error) {
	result := make([]plugin.InputBinding, 0, len(bindings))
	for _, path := range slices.Sorted(maps.Keys(bindings)) {
		value, err := bindings[path].domain()
		if err != nil {
			return nil, err
		}
		result = append(result, plugin.InputBinding{Path: path, Value: value})
	}
	return result, nil
}

func (w wireBinding) domain() (plugin.Binding, error) {
	sources := 0
	var binding plugin.Binding
	if len(w.Value) != 0 {
		literal, err := framevalue.ParseJSON(w.Value)
		if err != nil {
			return plugin.Binding{}, err
		}
		binding, sources = plugin.Binding{Source: extension.TerminalBindingValue, Literal: literal}, sources+1
	}
	if w.Row != nil {
		binding, sources = plugin.Binding{Source: extension.TerminalBindingRow, Field: *w.Row}, sources+1
	}
	if w.Context != nil {
		binding, sources = plugin.Binding{Source: extension.TerminalBindingContext, Context: *w.Context}, sources+1
	}
	if w.Form != nil {
		form := &plugin.FormField{Title: w.Form.Title}
		if w.Form.Default != nil {
			value, err := w.Form.Default.domain()
			if err != nil {
				return plugin.Binding{}, err
			}
			form.Default = &value
		}
		binding, sources = plugin.Binding{Source: extension.TerminalBindingForm, Form: form}, sources+1
	}
	if sources != 1 {
		return plugin.Binding{}, plugin.ErrInvalid
	}
	return binding, nil
}
