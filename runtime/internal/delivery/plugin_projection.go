package delivery

import (
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/protocol"
	"maps"
	"slices"
)

func presentPluginRequestGrant(v plugin.RequestGrant) protocol.PluginRequestGrant {
	result := protocol.PluginRequestGrant{Capability: string(v.Capability), Targets: slices.Clone(v.Targets)}
	return result
}
func presentPluginInput(v plugin.Input) protocol.PluginInput {
	result := protocol.PluginInput{ID: v.ID, Secret: v.Secret, Required: v.Required, Server: v.Server, Target: string(v.Target), Key: v.Key}
	return result
}
func presentPluginTheme(v plugin.Theme) protocol.PluginTheme {
	result := protocol.PluginTheme{ID: v.ID, Title: v.Title, Scheme: protocol.PluginThemeScheme(v.Scheme), Colors: maps.Clone(v.Colors)}
	return result
}
func presentPluginSkill(v plugin.Skill) protocol.PluginSkill {
	result := protocol.PluginSkill{Name: v.Name, Description: v.Description}
	return result
}
func presentPluginServerDeclaration(v plugin.Server) protocol.PluginServerDeclaration {
	result := protocol.PluginServerDeclaration{Name: v.Name, Type: string(v.Type), Command: v.Command, Args: slices.Clone(v.Args), Env: maps.Clone(v.Env), CWD: v.CWD, URL: v.URL, Headers: maps.Clone(v.Headers)}
	return result
}
func presentPluginDiagnostic(v plugin.Diagnostic) protocol.PluginDiagnostic {
	result := protocol.PluginDiagnostic{Component: v.Component, Code: v.Code}
	return result
}
func presentPluginRelease(v plugin.Release) protocol.PluginRelease {
	result := protocol.PluginRelease{Digest: v.Digest, Name: v.Name, Version: v.Version, Description: v.Description}
	result.Requests = make([]protocol.PluginRequestGrant, 0, len(v.Requests))
	for _, item := range v.Requests {
		result.Requests = append(result.Requests, presentPluginRequestGrant(item))
	}
	result.Servers = make([]protocol.PluginServerDeclaration, 0, len(v.Servers))
	for _, item := range v.Servers {
		result.Servers = append(result.Servers, presentPluginServerDeclaration(item))
	}
	result.Inputs = make([]protocol.PluginInput, 0, len(v.Inputs))
	for _, item := range v.Inputs {
		result.Inputs = append(result.Inputs, presentPluginInput(item))
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
		result.Diagnostics = append(result.Diagnostics, presentPluginDiagnostic(item))
	}
	return result
}
func presentInstallation(record plugin.Record) *protocol.PluginInstallation {
	result := &protocol.PluginInstallation{Availability: []protocol.PluginDiagnostic{}, ID: record.ID, Source: record.Source, Selected: presentPluginRelease(record.Selected), Enabled: record.Enabled, ApprovedDigest: record.ApprovedDigest, Values: maps.Clone(record.Values), DisabledServers: slices.Clone(record.DisabledServers), DisabledSkills: slices.Clone(record.DisabledSkills), Grants: make([]protocol.PluginRequestGrant, 0, len(record.Grants))}
	if record.Staged != nil {
		value := presentPluginRelease(*record.Staged)
		result.Staged = &value
	}
	for _, g := range record.Grants {
		result.Grants = append(result.Grants, presentPluginRequestGrant(g))
	}
	for _, input := range record.Selected.Inputs {
		if _, configured := result.Values[input.ID]; input.Secret && configured {
			result.Values[input.ID] = "[REDACTED]"
		}
	}
	return result
}
