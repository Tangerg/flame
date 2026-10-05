package delivery

import (
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/protocol"
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
	switch v.Transport {
	case mcpserver.TransportStdio:
		result.Type = protocol.MCPTransportStdio
		result.Command, result.Args, result.Env, result.Dir = v.Command, slices.Clone(v.Args), maps.Clone(v.Env), v.Dir
	case mcpserver.TransportStreamableHTTP:
		result.Type = protocol.MCPTransportStreamableHTTP
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
	record := inspection.Record
	realization, err := presentPluginRealization(inspection.Realization)
	if err != nil {
		return nil, err
	}
	presentation, err := presentPluginPresentation(inspection.Presentation)
	if err != nil {
		return nil, err
	}
	state, err := presentPluginInstallationState(record.State)
	if err != nil {
		return nil, err
	}
	disabledServers := make([]string, 0, len(record.DisabledServers))
	for _, name := range record.DisabledServers {
		disabledServers = append(disabledServers, name.String())
	}
	selected, err := presentPluginRelease(inspection.Selected)
	if err != nil {
		return nil, err
	}
	result := &protocol.PluginInstallation{Realization: realization, Presentation: presentation, ID: record.ID.String(), Source: record.Source, Selected: selected, State: state, InputStates: presentPluginInputStates(record, inspection.Selected), DisabledServers: disabledServers, DisabledSkills: slices.Clone(record.DisabledSkills)}
	if inspection.Staged != nil {
		staged, err := presentPluginRelease(*inspection.Staged)
		if err != nil {
			return nil, err
		}
		result.Staged = &staged
	}
	return result, nil
}

// presentPluginInputStates is the only projection of configured input values.
// A secret's text is not copied into the result at all, so no later encoder,
// log or client cache can carry it.
func presentPluginInputStates(record plugin.Record, selected plugin.Release) map[string]protocol.PluginInputState {
	inputs := selected.Declaration().Inputs
	states := make(map[string]protocol.PluginInputState, len(inputs))
	for _, input := range inputs {
		value, configured := record.Values[input.ID]
		switch {
		case !configured:
			states[input.ID] = protocol.PluginInputState{Type: protocol.PluginInputUnset}
		case input.Secret:
			states[input.ID] = protocol.PluginInputState{Type: protocol.PluginInputConfigured}
		default:
			states[input.ID] = protocol.PluginInputState{Type: protocol.PluginInputValue, Value: new(value)}
		}
	}
	return states
}

func presentPluginRealization(realization plugins.Realization) (protocol.PluginRealization, error) {
	switch realization.Release {
	case plugins.ReleaseAvailable:
		var backends []string
		for _, name := range realization.UnavailableBackends() {
			backends = append(backends, name.String())
		}
		return protocol.PluginRealization{Type: protocol.PluginRealizationAvailable, UnavailableBackends: backends}, nil
	case plugins.ReleaseUnavailable:
		return protocol.PluginRealization{Type: protocol.PluginRealizationReleaseUnavailable}, nil
	default:
		return protocol.PluginRealization{}, fmt.Errorf("plugin: project release state %q", realization.Release)
	}
}

func presentPluginInstallationState(state plugin.State) (protocol.PluginInstallationState, error) {
	switch state {
	case plugin.Unapproved:
		return protocol.PluginInstallationUnapproved, nil
	case plugin.Approved:
		return protocol.PluginInstallationApproved, nil
	case plugin.Enabled:
		return protocol.PluginInstallationEnabled, nil
	default:
		return "", fmt.Errorf("plugin: project installation state %q", state)
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
