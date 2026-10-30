package extension

import "slices"

type ComponentScope string

const (
	ComponentScopeInstallation ComponentScope = "installation"
	ComponentScopeWorkspace    ComponentScope = "workspace"
	ComponentScopeSession      ComponentScope = "session"
	ComponentScopeClient       ComponentScope = "client"
	ComponentScopeFile         ComponentScope = "file"
	ComponentScopeToolResult   ComponentScope = "toolResult"
)

func (ComponentScope) Values() []string {
	return []string{string(ComponentScopeInstallation), string(ComponentScopeWorkspace), string(ComponentScopeSession), string(ComponentScopeClient), string(ComponentScopeFile), string(ComponentScopeToolResult)}
}

func (s ComponentScope) Valid() bool { return slices.Contains(s.Values(), string(s)) }

type ComponentPlacement string

const (
	ComponentPlacementExport             ComponentPlacement = "export"
	ComponentPlacementWorkspaceView      ComponentPlacement = "workspaceView"
	ComponentPlacementSettingsPane       ComponentPlacement = "settingsPane"
	ComponentPlacementSessionAction      ComponentPlacement = "sessionAction"
	ComponentPlacementFileRenderer       ComponentPlacement = "fileRenderer"
	ComponentPlacementToolResultRenderer ComponentPlacement = "toolResultRenderer"
)

func (ComponentPlacement) Values() []string {
	return []string{string(ComponentPlacementExport), string(ComponentPlacementWorkspaceView), string(ComponentPlacementSettingsPane), string(ComponentPlacementSessionAction), string(ComponentPlacementFileRenderer), string(ComponentPlacementToolResultRenderer)}
}

type SettingsIntent string

const (
	SettingsIntentModelProviders SettingsIntent = "modelProviders"
	SettingsIntentMCPServers     SettingsIntent = "mcpServers"
)

func (SettingsIntent) Values() []string {
	return []string{string(SettingsIntentModelProviders), string(SettingsIntentMCPServers)}
}

func (s SettingsIntent) Valid() bool { return slices.Contains(s.Values(), string(s)) }

type RendererSelectorKind string

const (
	RendererToolName      RendererSelectorKind = "toolName"
	RendererResultShape   RendererSelectorKind = "resultShape"
	RendererFileExtension RendererSelectorKind = "fileExtension"
)

func (RendererSelectorKind) Values() []string {
	return []string{string(RendererToolName), string(RendererResultShape), string(RendererFileExtension)}
}
