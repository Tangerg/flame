package persistence

import (
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	framevalue "github.com/Tangerg/frame/value"
)

type storedDeclaration struct {
	Name          string               `json:"name"`
	Version       string               `json:"version"`
	Description   string               `json:"description"`
	Servers       []storedServer       `json:"servers"`
	Contributions []storedContribution `json:"contributions"`
	Points        []storedPoint        `json:"points"`
	PointValues   []storedPointValue   `json:"pointValues"`
	Components    []storedComponent    `json:"components"`
	TerminalViews []storedTerminalView `json:"terminalViews"`
	Inputs        []storedInput        `json:"inputs"`
	Themes        []storedTheme        `json:"themes"`
	Locales       []storedLocale       `json:"locales"`
	Skills        []storedSkill        `json:"skills"`
	Diagnostics   []storedDiagnostic   `json:"diagnostics"`
}

type storedServer struct {
	Name      mcpserver.ServerName  `json:"name"`
	Managed   *storedManagedBackend `json:"managed,omitzero"`
	Transport mcpserver.Transport   `json:"transport"`
	Command   string                `json:"command"`
	Args      []string              `json:"args"`
	Env       map[string]string     `json:"env"`
	Dir       string                `json:"dir"`
	URL       string                `json:"url"`
	Headers   map[string]string     `json:"headers"`
}

type storedManagedBackend struct {
	Runtime plugin.ManagedRuntime `json:"runtime"`
	Entry   string                `json:"entry"`
}

type storedContribution struct {
	ModelProviders []string              `json:"modelProviders"`
	Background     bool                  `json:"background"`
	ID             plugin.ContributionID `json:"id"`
	Server         mcpserver.ServerName  `json:"server"`
	Export         string                `json:"export"`
	Contract       framevalue.JSON       `json:"contract"`
	Requires       []storedRequirement   `json:"requires"`
}

type storedRequirement struct {
	ID       plugin.ContributionID `json:"id"`
	Contract framevalue.JSON       `json:"contract"`
	Optional bool                  `json:"optional"`
}

type storedPoint struct {
	ID       plugin.ContributionID `json:"id"`
	Contract framevalue.JSON       `json:"contract"`
}

type storedPointValue struct {
	ID       plugin.ContributionID `json:"id"`
	Contract framevalue.JSON       `json:"contract"`
	Value    framevalue.JSON       `json:"value"`
}

type storedComponent struct {
	ToolCategory    builtin.ToolCategory         `json:"toolCategory"`
	TargetView      string                       `json:"targetView"`
	Selector        *storedRendererSelector      `json:"selector,omitzero"`
	ID              plugin.ContributionID        `json:"id"`
	Title           string                       `json:"title"`
	LocalizedTitles map[string]string            `json:"localizedTitles"`
	Scope           extension.ComponentScope     `json:"scope"`
	Placement       extension.ComponentPlacement `json:"placement"`
	Entry           string                       `json:"entry"`
	Export          string                       `json:"export"`
	Contribution    plugin.ContributionID        `json:"contribution"`
	Material        string                       `json:"material"`
	SettingsIntent  string                       `json:"settingsIntent"`
}

type storedRendererSelector struct {
	Kind  extension.RendererSelectorKind `json:"kind"`
	Name  string                         `json:"name"`
	Shape extension.ResultShape          `json:"shape"`
}

type storedInput struct {
	ID       string               `json:"id"`
	Secret   bool                 `json:"secret"`
	Required bool                 `json:"required"`
	Server   mcpserver.ServerName `json:"server"`
	Target   plugin.InputTarget   `json:"target"`
	Key      string               `json:"key"`
}

type storedTheme struct {
	ID     string             `json:"id"`
	Title  string             `json:"title"`
	Scheme plugin.ThemeScheme `json:"scheme"`
	Colors storedThemeColors  `json:"colors"`
}

type storedThemeColors struct {
	Background string `json:"background"`
	Foreground string `json:"foreground"`
	Accent     string `json:"accent"`
	Muted      string `json:"muted"`
	Border     string `json:"border"`
}

type storedLocale struct {
	Tag   string `json:"tag"`
	Title string `json:"title"`
	Entry string `json:"entry"`
}

type storedSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type storedDiagnostic struct {
	Component storedDiagnosticComponent `json:"component"`
	Code      plugin.DiagnosticCode     `json:"code"`
}

type storedDiagnosticComponent struct {
	Kind plugin.ComponentKind `json:"kind"`
	Name string               `json:"name"`
}

type storedTerminalView struct {
	ID           string                 `json:"id"`
	Title        string                 `json:"title"`
	Contribution plugin.ContributionID  `json:"contribution"`
	List         storedTerminalList     `json:"list"`
	Actions      []storedTerminalAction `json:"actions"`
}

type storedTerminalList struct {
	Method  string                 `json:"method"`
	Input   []storedInputBinding   `json:"input"`
	Rows    string                 `json:"rows"`
	Key     string                 `json:"key"`
	Paging  *storedTerminalPaging  `json:"paging,omitzero"`
	Columns []storedTerminalColumn `json:"columns"`
}

type storedTerminalPaging struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

type storedTerminalColumn struct {
	Title string `json:"title"`
	Field string `json:"field"`
}

type storedTerminalAction struct {
	ID      string                         `json:"id"`
	Title   string                         `json:"title"`
	Method  string                         `json:"method"`
	Target  extension.TerminalActionTarget `json:"target"`
	Confirm bool                           `json:"confirm"`
	Input   []storedInputBinding           `json:"input"`
}

type storedInputBinding struct {
	Path  string        `json:"path"`
	Value storedBinding `json:"value"`
}

type storedBinding struct {
	Source  extension.TerminalBindingSource `json:"source"`
	Literal *framevalue.JSON                `json:"literal,omitzero"`
	Field   string                          `json:"field"`
	Context extension.TerminalContext       `json:"context"`
	Form    *storedFormField                `json:"form,omitzero"`
}

type storedFormField struct {
	Title   string                     `json:"title"`
	Default *storedBinding             `json:"default,omitzero"`
	Kind    extension.TerminalFormKind `json:"kind"`
	Options []string                   `json:"options"`
}

func convertEach[T, U any](values []T, convert func(T) U) []U {
	if values == nil {
		return nil
	}
	converted := make([]U, len(values))
	for index, value := range values {
		converted[index] = convert(value)
	}
	return converted
}

func storeDeclaration(declaration plugin.Declaration) storedDeclaration {
	return storedDeclaration{
		Name:        declaration.Name,
		Version:     declaration.Version,
		Description: declaration.Description,
		Servers:     convertEach(declaration.Servers, storeServer),
		Contributions: convertEach(declaration.Contributions, func(value plugin.Contribution) storedContribution {
			return storedContribution{
				ModelProviders: value.ModelProviders, Background: value.Background, ID: value.ID, Server: value.Server,
				Export: value.Export, Contract: value.Contract,
				Requires: convertEach(value.Requires, func(value plugin.Requirement) storedRequirement { return storedRequirement(value) }),
			}
		}),
		Points:        convertEach(declaration.Points, func(value plugin.Point) storedPoint { return storedPoint(value) }),
		PointValues:   convertEach(declaration.PointValues, func(value plugin.PointValue) storedPointValue { return storedPointValue(value) }),
		Components:    convertEach(declaration.Components, storeComponent),
		TerminalViews: convertEach(declaration.TerminalViews, storeTerminalView),
		Inputs:        convertEach(declaration.Inputs, func(value plugin.Input) storedInput { return storedInput(value) }),
		Themes: convertEach(declaration.Themes, func(value plugin.Theme) storedTheme {
			return storedTheme{ID: value.ID, Title: value.Title, Scheme: value.Scheme, Colors: storedThemeColors(value.Colors)}
		}),
		Locales: convertEach(declaration.Locales, func(value plugin.Locale) storedLocale { return storedLocale(value) }),
		Skills:  convertEach(declaration.Skills, func(value plugin.Skill) storedSkill { return storedSkill(value) }),
		Diagnostics: convertEach(declaration.Diagnostics, func(value plugin.Diagnostic) storedDiagnostic {
			return storedDiagnostic{Component: storedDiagnosticComponent(value.Component), Code: value.Code}
		}),
	}
}

func (s storedDeclaration) declaration() (plugin.Declaration, error) {
	servers := make([]plugin.Server, len(s.Servers))
	for index, record := range s.Servers {
		server, err := record.server()
		if err != nil {
			return plugin.Declaration{}, err
		}
		servers[index] = server
	}
	if s.Servers == nil {
		servers = nil
	}
	return plugin.Declaration{
		Name:        s.Name,
		Version:     s.Version,
		Description: s.Description,
		Servers:     servers,
		Contributions: convertEach(s.Contributions, func(value storedContribution) plugin.Contribution {
			return plugin.Contribution{
				ModelProviders: value.ModelProviders, Background: value.Background, ID: value.ID, Server: value.Server,
				Export: value.Export, Contract: value.Contract,
				Requires: convertEach(value.Requires, func(value storedRequirement) plugin.Requirement { return plugin.Requirement(value) }),
			}
		}),
		Points:        convertEach(s.Points, func(value storedPoint) plugin.Point { return plugin.Point(value) }),
		PointValues:   convertEach(s.PointValues, func(value storedPointValue) plugin.PointValue { return plugin.PointValue(value) }),
		Components:    convertEach(s.Components, storedComponent.component),
		TerminalViews: convertEach(s.TerminalViews, storedTerminalView.view),
		Inputs:        convertEach(s.Inputs, func(value storedInput) plugin.Input { return plugin.Input(value) }),
		Themes: convertEach(s.Themes, func(value storedTheme) plugin.Theme {
			return plugin.Theme{ID: value.ID, Title: value.Title, Scheme: value.Scheme, Colors: plugin.ThemeColors(value.Colors)}
		}),
		Locales: convertEach(s.Locales, func(value storedLocale) plugin.Locale { return plugin.Locale(value) }),
		Skills:  convertEach(s.Skills, func(value storedSkill) plugin.Skill { return plugin.Skill(value) }),
		Diagnostics: convertEach(s.Diagnostics, func(value storedDiagnostic) plugin.Diagnostic {
			return plugin.Diagnostic{Component: plugin.Component(value.Component), Code: value.Code}
		}),
	}, nil
}

func storeServer(server plugin.Server) storedServer {
	record := storedServer{Name: server.Name, Transport: server.Transport()}
	switch startup := server.Startup.(type) {
	case plugin.ManagedBackend:
		managed := storedManagedBackend(startup)
		record.Managed = &managed
	case plugin.DeclaredConnection:
		switch connection := startup.Connection.(type) {
		case mcpserver.StdioConnection:
			record.Command, record.Args, record.Env, record.Dir = connection.Command, connection.Args, connection.Env, connection.Dir
		case mcpserver.HTTPConnection:
			record.URL, record.Headers = connection.URL, connection.Headers
		}
	}
	return record
}

func (s storedServer) server() (plugin.Server, error) {
	server := plugin.Server{Name: s.Name}
	switch {
	case s.Managed != nil:
		server.Startup = plugin.ManagedBackend(*s.Managed)
	case s.Transport == mcpserver.TransportStdio:
		server.Startup = plugin.DeclaredConnection{Connection: mcpserver.StdioConnection{Command: s.Command, Args: s.Args, Env: s.Env, Dir: s.Dir}}
	case s.Transport == mcpserver.TransportStreamableHTTP:
		server.Startup = plugin.DeclaredConnection{Connection: mcpserver.HTTPConnection{URL: s.URL, Headers: s.Headers}}
	default:
		return plugin.Server{}, fmt.Errorf("server %q has unknown transport %q", s.Name, s.Transport)
	}
	return server, nil
}

func storeComponent(component plugin.ClientComponent) storedComponent {
	stored := storedComponent{
		ToolCategory: component.ToolCategory, TargetView: component.TargetView, ID: component.ID, Title: component.Title,
		LocalizedTitles: component.LocalizedTitles, Scope: component.Scope, Placement: component.Placement, Entry: component.Entry,
		Export: component.Export, Contribution: component.Contribution, Material: component.Material, SettingsIntent: component.SettingsIntent,
	}
	if component.Selector != nil {
		selector := storedRendererSelector(*component.Selector)
		stored.Selector = &selector
	}
	return stored
}

func (s storedComponent) component() plugin.ClientComponent {
	component := plugin.ClientComponent{
		ToolCategory: s.ToolCategory, TargetView: s.TargetView, ID: s.ID, Title: s.Title,
		LocalizedTitles: s.LocalizedTitles, Scope: s.Scope, Placement: s.Placement, Entry: s.Entry,
		Export: s.Export, Contribution: s.Contribution, Material: s.Material, SettingsIntent: s.SettingsIntent,
	}
	if s.Selector != nil {
		selector := plugin.RendererSelector(*s.Selector)
		component.Selector = &selector
	}
	return component
}

func storeTerminalView(view plugin.TerminalView) storedTerminalView {
	list := storedTerminalList{
		Method: view.List.Method, Input: convertEach(view.List.Input, storeInputBinding), Rows: view.List.Rows, Key: view.List.Key,
		Columns: convertEach(view.List.Columns, func(value plugin.TerminalColumn) storedTerminalColumn { return storedTerminalColumn(value) }),
	}
	if view.List.Paging != nil {
		paging := storedTerminalPaging(*view.List.Paging)
		list.Paging = &paging
	}
	return storedTerminalView{
		ID: view.ID, Title: view.Title, Contribution: view.Contribution, List: list,
		Actions: convertEach(view.Actions, func(action plugin.TerminalAction) storedTerminalAction {
			return storedTerminalAction{ID: action.ID, Title: action.Title, Method: action.Method, Target: action.Target, Confirm: action.Confirm, Input: convertEach(action.Input, storeInputBinding)}
		}),
	}
}

func (s storedTerminalView) view() plugin.TerminalView {
	list := plugin.TerminalList{
		Method: s.List.Method, Input: convertEach(s.List.Input, storedInputBinding.binding), Rows: s.List.Rows, Key: s.List.Key,
		Columns: convertEach(s.List.Columns, func(value storedTerminalColumn) plugin.TerminalColumn { return plugin.TerminalColumn(value) }),
	}
	if s.List.Paging != nil {
		paging := plugin.TerminalPaging(*s.List.Paging)
		list.Paging = &paging
	}
	return plugin.TerminalView{
		ID: s.ID, Title: s.Title, Contribution: s.Contribution, List: list,
		Actions: convertEach(s.Actions, func(action storedTerminalAction) plugin.TerminalAction {
			return plugin.TerminalAction{ID: action.ID, Title: action.Title, Method: action.Method, Target: action.Target, Confirm: action.Confirm, Input: convertEach(action.Input, storedInputBinding.binding)}
		}),
	}
}

func storeInputBinding(binding plugin.InputBinding) storedInputBinding {
	return storedInputBinding{Path: binding.Path, Value: storeBinding(binding.Value)}
}

func (s storedInputBinding) binding() plugin.InputBinding {
	return plugin.InputBinding{Path: s.Path, Value: s.Value.binding()}
}

func storeBinding(binding plugin.Binding) storedBinding {
	stored := storedBinding{Source: binding.Source, Field: binding.Field, Context: binding.Context}
	if len(binding.Literal.Bytes()) != 0 {
		stored.Literal = &binding.Literal
	}
	if binding.Form != nil {
		form := storedFormField{Title: binding.Form.Title, Kind: binding.Form.Kind, Options: binding.Form.Options}
		if binding.Form.Default != nil {
			value := storeBinding(*binding.Form.Default)
			form.Default = &value
		}
		stored.Form = &form
	}
	return stored
}

func (s storedBinding) binding() plugin.Binding {
	binding := plugin.Binding{Source: s.Source, Field: s.Field, Context: s.Context}
	if s.Literal != nil {
		binding.Literal = *s.Literal
	}
	if s.Form != nil {
		form := plugin.FormField{Title: s.Form.Title, Kind: s.Form.Kind, Options: s.Form.Options}
		if s.Form.Default != nil {
			value := s.Form.Default.binding()
			form.Default = &value
		}
		binding.Form = &form
	}
	return binding
}
