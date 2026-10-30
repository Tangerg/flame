package delivery

import (
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func presentPluginInput(v plugin.Input) (protocol.PluginInput, error) {
	result := protocol.PluginInput{ID: v.ID, Secret: v.Secret, Required: v.Required, Server: v.Server.String(), Key: v.Key}
	switch v.Target {
	case plugin.Environment:
		result.Target = protocol.PluginInputEnvironment
	case plugin.Header:
		result.Target = protocol.PluginInputHeader
	case plugin.Authorization:
		result.Target = protocol.PluginInputAuthorization
	default:
		return protocol.PluginInput{}, fmt.Errorf("plugin: project input target %q", v.Target)
	}
	return result, nil
}
func presentPluginTheme(v plugin.Theme) protocol.PluginTheme {
	colors := protocol.PluginThemeColors{Background: v.Colors.Background, Foreground: v.Colors.Foreground, Accent: v.Colors.Accent, Muted: v.Colors.Muted, Border: v.Colors.Border}
	return protocol.PluginTheme{ID: v.ID, Title: v.Title, Scheme: protocol.PluginThemeScheme(v.Scheme), Colors: colors}
}
func presentPluginSkill(v plugin.Skill) protocol.PluginSkill {
	result := protocol.PluginSkill{Name: v.Name, Description: v.Description}
	return result
}
func presentPluginServerDeclaration(v plugin.Server) (protocol.PluginServerDeclaration, error) {
	result := protocol.PluginServerDeclaration{Name: v.Name.String()}
	if v.Managed != nil {
		result.Type = protocol.PluginServerNode
		result.Entry = v.Managed.Entry
		return result, nil
	}
	switch v.Transport {
	case mcpserver.TransportStdio:
		result.Type = protocol.PluginServerStdio
		result.Command, result.Args, result.Env, result.Dir = v.Command, slices.Clone(v.Args), maps.Clone(v.Env), v.Dir
	case mcpserver.TransportStreamableHTTP:
		result.Type = protocol.PluginServerHTTP
		result.URL, result.Headers = v.URL, maps.Clone(v.Headers)
	default:
		return protocol.PluginServerDeclaration{}, fmt.Errorf("plugin: project server transport %q", v.Transport)
	}
	return result, nil
}
func presentPluginDiagnostic(v plugin.Diagnostic) (protocol.PluginDiagnostic, error) {
	named, err := v.Component.Kind.Named()
	if err != nil {
		return protocol.PluginDiagnostic{}, err
	}
	component := protocol.PluginComponent{Type: protocol.PluginComponentType(v.Component.Kind)}
	if named {
		component.Name = new(v.Component.Name)
	}
	return protocol.PluginDiagnostic{Component: component, Code: protocol.PluginDiagnosticCode(v.Code)}, nil
}
func presentPluginRelease(release plugin.Release) (protocol.PluginRelease, error) {
	v := release.Declaration()
	result := protocol.PluginRelease{Digest: release.Digest().String(), Name: v.Name, Version: v.Version, Description: v.Description}
	result.Components = make([]protocol.PluginClientComponent, 0, len(v.Components))
	for _, value := range v.Components {
		var selector *protocol.PluginRendererSelector
		if value.Selector != nil {
			selector = &protocol.PluginRendererSelector{Type: value.Selector.Kind, Name: value.Selector.Name}
		}
		result.Components = append(result.Components, protocol.PluginClientComponent{Selector: selector, ID: value.ID.String(), Title: value.Title, Scope: value.Scope, Placement: value.Placement, Entry: value.Entry, Export: value.Export, Contribution: value.Contribution.String(), Material: value.Material, SettingsIntent: value.SettingsIntent})
	}
	result.Points = make([]protocol.PluginPoint, 0, len(v.Points))
	for _, value := range v.Points {
		result.Points = append(result.Points, protocol.PluginPoint{ID: value.ID.String(), Contract: string(value.Contract.JSON())})
	}
	result.PointValues = make([]protocol.PluginPointValue, 0, len(v.PointValues))
	for _, value := range v.PointValues {
		result.PointValues = append(result.PointValues, protocol.PluginPointValue{ID: value.ID.String(), Contract: string(value.Contract.JSON()), Value: string(value.Value.JSON())})
	}
	result.Contributions = make([]protocol.PluginContribution, 0, len(v.Contributions))
	for _, value := range v.Contributions {
		presented := protocol.PluginContribution{Background: value.Background, ID: value.ID.String(), Server: value.Server.String(), Export: value.Export, Contract: string(value.Contract.JSON()), Requires: []protocol.PluginRequirement{}}
		for _, requirement := range value.Requires {
			presented.Requires = append(presented.Requires, protocol.PluginRequirement{ID: requirement.ID.String(), Contract: string(requirement.Contract.JSON()), Optional: requirement.Optional})
		}
		result.Contributions = append(result.Contributions, presented)
	}
	result.Servers = make([]protocol.PluginServerDeclaration, 0, len(v.Servers))
	for _, item := range v.Servers {
		server, err := presentPluginServerDeclaration(item)
		if err != nil {
			return protocol.PluginRelease{}, err
		}
		result.Servers = append(result.Servers, server)
	}
	result.Inputs = make([]protocol.PluginInput, 0, len(v.Inputs))
	for _, item := range v.Inputs {
		input, err := presentPluginInput(item)
		if err != nil {
			return protocol.PluginRelease{}, err
		}
		result.Inputs = append(result.Inputs, input)
	}
	result.Themes = make([]protocol.PluginTheme, 0, len(v.Themes))
	for _, item := range v.Themes {
		result.Themes = append(result.Themes, presentPluginTheme(item))
	}
	result.Locales = make([]protocol.PluginLocale, 0, len(v.Locales))
	for _, item := range v.Locales {
		result.Locales = append(result.Locales, protocol.PluginLocale{Tag: item.Tag, Title: item.Title})
	}
	result.Skills = make([]protocol.PluginSkill, 0, len(v.Skills))

	for _, item := range v.Skills {
		result.Skills = append(result.Skills, presentPluginSkill(item))
	}
	result.Diagnostics = make([]protocol.PluginDiagnostic, 0, len(v.Diagnostics))
	for _, item := range v.Diagnostics {
		diagnostic, err := presentPluginDiagnostic(item)
		if err != nil {
			return protocol.PluginRelease{}, err
		}
		result.Diagnostics = append(result.Diagnostics, diagnostic)
	}
	return result, nil
}
func presentInstallation(inspection plugins.Inspection) (*protocol.PluginInstallation, error) {
	view := inspection.View
	realization, err := presentPluginRealization(inspection.Realization)
	if err != nil {
		return nil, err
	}
	presentation, err := presentPluginPresentation(inspection.Presentation)
	if err != nil {
		return nil, err
	}
	inputStates, err := presentPluginInputStates(view.Inputs)
	if err != nil {
		return nil, err
	}
	disabledServers := make([]string, 0, len(view.DisabledServers))
	for _, name := range view.DisabledServers {
		disabledServers = append(disabledServers, name.String())
	}
	selected, err := presentPluginRelease(inspection.Selected)
	if err != nil {
		return nil, err
	}
	result := &protocol.PluginInstallation{Realization: realization, Presentation: presentation, ID: view.ID.String(), Source: view.Source, Selected: selected, Enabled: view.Enabled, InputStates: inputStates, DisabledServers: disabledServers, DisabledSkills: slices.Clone(view.DisabledSkills)}
	if inspection.Staged != nil {
		staged, err := presentPluginRelease(*inspection.Staged)
		if err != nil {
			return nil, err
		}
		result.Staged = &staged
	}
	return result, nil
}

func presentPluginInputStates(inputs map[string]plugin.InputState) (map[string]protocol.PluginInputState, error) {
	states := make(map[string]protocol.PluginInputState, len(inputs))
	for id, input := range inputs {
		switch input.Presence() {
		case plugin.InputUnset:
			states[id] = protocol.PluginInputState{Type: protocol.PluginInputUnset}
		case plugin.InputConfigured:
			states[id] = protocol.PluginInputState{Type: protocol.PluginInputConfigured}
		case plugin.InputValue:
			value, _ := input.Value()
			states[id] = protocol.PluginInputState{Type: protocol.PluginInputValue, Value: new(value)}
		default:
			return nil, fmt.Errorf("plugin: project input presence %q", input.Presence())
		}
	}
	return states, nil
}

func presentPluginRealization(realization plugins.Realization) (protocol.PluginRealization, error) {
	switch realization.Release {
	case plugins.ReleaseAvailable:
		var backends []string
		for _, name := range realization.UnavailableBackends {
			backends = append(backends, name.String())
		}
		return protocol.PluginRealization{Type: protocol.PluginRealizationAvailable, UnavailableBackends: backends}, nil
	case plugins.ReleaseUnavailable:
		return protocol.PluginRealization{Type: protocol.PluginRealizationReleaseUnavailable}, nil
	default:
		return protocol.PluginRealization{}, fmt.Errorf("plugin: project release state %q", realization.Release)
	}
}

func presentPluginPresentation(presentation plugins.Presentation) (protocol.PluginPresentation, error) {
	switch presentation {
	case plugins.PresentationAdmitted:
		return protocol.PluginPresentationAdmitted, nil
	case plugins.PresentationWithheld:
		return protocol.PluginPresentationWithheld, nil
	default:
		return "", fmt.Errorf("plugin: project presentation %q", presentation)
	}
}
