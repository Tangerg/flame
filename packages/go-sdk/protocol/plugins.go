package protocol

import (
	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/extension"

	framecall "github.com/Tangerg/frame/call"
)

type PluginCommandRequest struct {
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
}

// Contracts and feature values cross Core as JSON text because their numeric
// ranges and receipt lexemes belong to the feature, not to a JS number.
type CallPluginRequest struct {
	InstallationID string           `json:"installationId"`
	Digest         string           `json:"digest"`
	Contribution   string           `json:"contribution"`
	Contract       string           `json:"contract"`
	Method         string           `json:"method"`
	Input          string           `json:"input"`
	Target         framecall.Target `json:"target"`
}

type CallPluginResponse struct {
	Output  string             `json:"output,omitzero"`
	Failure *PluginCallFailure `json:"failure,omitzero"`
}

const MaximumPluginFailureCodeCharacters = 256

type PluginCallFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

//flame:enum
type PluginStreamKind string

const (
	PluginStreamChunk   PluginStreamKind = "chunk"
	PluginStreamResult  PluginStreamKind = "result"
	PluginStreamFailure PluginStreamKind = "failure"
)

type PluginStreamEvent struct {
	Kind    PluginStreamKind   `json:"kind"`
	Value   string             `json:"value,omitzero"`
	Failure *PluginCallFailure `json:"failure,omitzero"`
}

type PluginProviderSelection struct {
	Role           string `json:"role"`
	InstallationID string `json:"installationId"`
	Contribution   string `json:"contribution"`
}

type PluginComposition struct {
	Contract string                    `json:"contract"`
	Members  []PluginCompositionMember `json:"members"`
}

type PluginCompositionMember struct {
	InstallationID string `json:"installationId"`
	Contribution   string `json:"contribution"`
}

type SetPluginCompositionRequest struct {
	Contract string                    `json:"contract"`
	Members  []PluginCompositionMember `json:"members"`
}

type ResolvePluginCompositionRequest struct {
	Contract string `json:"contract"`
}

type SelectPluginProviderRequest struct {
	Role           string `json:"role"`
	InstallationID string `json:"installationId"`
	Contribution   string `json:"contribution"`
}

type PluginInstallation struct {
	Realization     PluginRealization           `json:"realization"`
	Presentation    PluginPresentation          `json:"presentation"`
	ID              string                      `json:"id"`
	Source          string                      `json:"source"`
	Selected        PluginRelease               `json:"selected"`
	Staged          *PluginRelease              `json:"staged,omitzero"`
	Enabled         bool                        `json:"enabled"`
	InputStates     map[string]PluginInputState `json:"inputStates"`
	DisabledServers []string                    `json:"disabledServers"`
	DisabledSkills  []string                    `json:"disabledSkills"`
}

//flame:enum
type PluginPresentation string

const (
	PluginPresentationAdmitted PluginPresentation = "admitted"
	PluginPresentationWithheld PluginPresentation = "withheld"
)

//flame:enum
type PluginRealizationType string

const (
	PluginRealizationAvailable          PluginRealizationType = "available"
	PluginRealizationReleaseUnavailable PluginRealizationType = "releaseUnavailable"
)

type PluginRealization struct {
	Type                PluginRealizationType `json:"type"`
	UnavailableBackends []string              `json:"unavailableBackends,omitzero"`
}

//flame:enum
type PluginInputStateType string

const (
	PluginInputUnset      PluginInputStateType = "unset"
	PluginInputConfigured PluginInputStateType = "configured"
	PluginInputValue      PluginInputStateType = "value"
)

type PluginInputState struct {
	Type  PluginInputStateType `json:"type"`
	Value *string              `json:"value,omitzero"`
}
type PluginRelease struct {
	Digest        string                    `json:"digest"`
	Name          string                    `json:"name"`
	Version       string                    `json:"version,omitempty"`
	Description   string                    `json:"description,omitempty"`
	Contributions []PluginContribution      `json:"contributions"`
	Points        []PluginPoint             `json:"points"`
	PointValues   []PluginPointValue        `json:"pointValues"`
	Components    []PluginClientComponent   `json:"components"`
	TerminalViews []PluginTerminalView      `json:"terminalViews"`
	Servers       []PluginServerDeclaration `json:"servers"`
	Inputs        []PluginInput             `json:"inputs"`
	Themes        []PluginTheme             `json:"themes"`
	Locales       []PluginLocale            `json:"locales"`
	Skills        []PluginSkill             `json:"skills"`
	Diagnostics   []PluginDiagnostic        `json:"diagnostics"`
}

type PluginRendererSelector struct {
	Type  extension.RendererSelectorKind `json:"type"`
	Name  string                         `json:"name,omitempty"`
	Shape extension.ResultShape          `json:"shape,omitempty"`
}

//flame:enum
type PluginServerType string

const (
	PluginServerStdio PluginServerType = "stdio"
	PluginServerHTTP  PluginServerType = "streamableHttp"
	PluginServerNode  PluginServerType = "managedNode"
)

type PluginServerDeclaration struct {
	Name    string            `json:"name"`
	Type    PluginServerType  `json:"type"`
	Entry   string            `json:"entry,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Dir     string            `json:"dir,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type PluginInput struct {
	ID       string            `json:"id"`
	Secret   bool              `json:"secret"`
	Required bool              `json:"required"`
	Server   string            `json:"server"`
	Target   PluginInputTarget `json:"target"`
	Key      string            `json:"key,omitempty"`
}

//flame:enum
type PluginInputTarget string

const (
	PluginInputEnvironment   PluginInputTarget = "env"
	PluginInputHeader        PluginInputTarget = "header"
	PluginInputAuthorization PluginInputTarget = "authorization"
)

type PluginTheme struct {
	ID     string            `json:"id"`
	Title  string            `json:"title"`
	Scheme PluginThemeScheme `json:"scheme"`
	Colors PluginThemeColors `json:"colors"`
}

type PluginLocale struct {
	Tag   string `json:"tag"`
	Title string `json:"title"`
}

type ReadPluginLocaleRequest struct {
	PluginReleaseRequest
	Tag string `json:"tag"`
}

type PluginLocaleCatalog struct {
	Messages map[string]string `json:"messages"`
}

type PluginThemeColors struct {
	Background string `json:"background,omitempty"`
	Foreground string `json:"foreground,omitempty"`
	Accent     string `json:"accent,omitempty"`
	Muted      string `json:"muted,omitempty"`
	Border     string `json:"border,omitempty"`
}

//flame:enum
type PluginThemeScheme string

const (
	PluginThemeDark  PluginThemeScheme = "dark"
	PluginThemeLight PluginThemeScheme = "light"
)

type PluginSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type PluginDiagnostic struct {
	Component PluginComponent      `json:"component"`
	Code      PluginDiagnosticCode `json:"code"`
}

//flame:enum
type PluginDiagnosticCode string

const (
	PluginDiagnosticUnknownField            PluginDiagnosticCode = "unknownField"
	PluginDiagnosticInvalidDeclaration      PluginDiagnosticCode = "invalidDeclaration"
	PluginDiagnosticUnsupportedContribution PluginDiagnosticCode = "unsupportedContribution"
	PluginDiagnosticComponentLimit          PluginDiagnosticCode = "componentLimit"
	PluginDiagnosticInvalidDependencies     PluginDiagnosticCode = "invalidDependencies"
	PluginDiagnosticUnavailableComponent    PluginDiagnosticCode = "unavailableComponent"
)

//flame:enum
type PluginComponentType string

const (
	PluginComponentManifestField  PluginComponentType = "manifestField"
	PluginComponentFlameExtension PluginComponentType = "flameExtension"
	PluginComponentExtensionField PluginComponentType = "extensionField"
	PluginComponentContribution   PluginComponentType = "contribution"
	PluginComponentMCP            PluginComponentType = "mcp"
	PluginComponentMCPServer      PluginComponentType = "mcpServer"
	PluginComponentSkills         PluginComponentType = "skills"
	PluginComponentSkill          PluginComponentType = "skill"
)

type PluginComponent struct {
	Type PluginComponentType `json:"type"`
	Name *string             `json:"name,omitzero"`
}
type InstallPluginRequest struct {
	Source string `json:"source"`
}
type StagePluginRequest struct {
	InstallationID string `json:"installationId"`
	Source         string `json:"source"`
}
type PluginRequest struct {
	InstallationID string `json:"installationId"`
}
type PluginReleaseRequest struct {
	InstallationID string `json:"installationId"`
	Digest         string `json:"digest"`
}
type SetPluginEnablementRequest struct {
	InstallationID string `json:"installationId"`
	Enabled        bool   `json:"enabled"`
}

type ConfigurePluginRequest struct {
	InstallationID string                           `json:"installationId"`
	Digest         string                           `json:"digest"`
	ValueChanges   map[string]PluginValueChange     `json:"valueChanges"`
	ServerChanges  map[string]PluginComponentChange `json:"serverChanges"`
	SkillChanges   map[string]PluginComponentChange `json:"skillChanges"`
}

//flame:enum
type PluginComponentChange string

const (
	PluginComponentEnable  PluginComponentChange = "enable"
	PluginComponentDisable PluginComponentChange = "disable"
)

//flame:enum
type PluginValueChangeType string

const (
	PluginValueSet   PluginValueChangeType = "set"
	PluginValueClear PluginValueChangeType = "clear"
)

type PluginValueChange struct {
	Type  PluginValueChangeType `json:"type"`
	Value *string               `json:"value,omitzero"`
}

type PluginContribution struct {
	SelectionRoles []string            `json:"selectionRoles,omitzero"`
	Background     bool                `json:"background,omitzero"`
	ID             string              `json:"id"`
	Server         string              `json:"server"`
	Export         string              `json:"export"`
	Contract       string              `json:"contract"`
	Requires       []PluginRequirement `json:"requires"`
}

type PluginRequirement struct {
	ID       string `json:"id"`
	Contract string `json:"contract"`
	Optional bool   `json:"optional"`
}

type ResolvePluginRequirementsRequest struct {
	InstallationID string `json:"installationId"`
	Digest         string `json:"digest"`
	Contribution   string `json:"contribution"`
}

type PluginCollaborator struct {
	ID       string                   `json:"id"`
	Bindings []PluginOperationBinding `json:"bindings"`
}

type PluginOperationBinding struct {
	InstallationID string `json:"installationId"`
	Digest         string `json:"digest"`
	Contribution   string `json:"contribution"`
	Contract       string `json:"contract"`
}

type PluginPoint struct {
	ID       string `json:"id"`
	Contract string `json:"contract"`
}

type PluginPointValue struct {
	ID       string `json:"id"`
	Contract string `json:"contract"`
	Value    string `json:"value"`
}

type PluginPointReference struct {
	InstallationID string `json:"installationId"`
	Contribution   string `json:"contribution"`
}

type ResolvePluginPointRequest struct {
	Contract   string                 `json:"contract"`
	Target     framecall.Target       `json:"target"`
	Attributes string                 `json:"attributes"`
	Order      []PluginPointReference `json:"order"`
}

type PluginPointBinding struct {
	InstallationID string `json:"installationId"`
	Digest         string `json:"digest"`
	Contribution   string `json:"contribution"`
	Value          string `json:"value"`
}

type PluginClientComponent struct {
	ToolCategory builtin.ToolCategory `json:"toolCategory,omitzero"`
	TargetView   string               `json:"targetView,omitzero"`

	Selector        *PluginRendererSelector      `json:"selector,omitzero"`
	ID              string                       `json:"id"`
	Title           string                       `json:"title"`
	LocalizedTitles map[string]string            `json:"localizedTitles,omitzero"`
	Scope           extension.ComponentScope     `json:"scope"`
	Placement       extension.ComponentPlacement `json:"placement"`
	Entry           string                       `json:"entry"`
	Export          string                       `json:"export"`
	Contribution    string                       `json:"contribution,omitzero"`
	Material        string                       `json:"material,omitzero"`
	SettingsIntent  string                       `json:"settingsIntent,omitzero"`
}

type PluginTerminalView struct {
	ID           string                 `json:"id"`
	Title        string                 `json:"title"`
	Contribution string                 `json:"contribution"`
	List         PluginTerminalList     `json:"list"`
	Actions      []PluginTerminalAction `json:"actions"`
}

type PluginTerminalList struct {
	Method  string                 `json:"method"`
	Input   []PluginTerminalInput  `json:"input"`
	Rows    string                 `json:"rows"`
	Key     string                 `json:"key"`
	Paging  *PluginTerminalPaging  `json:"paging,omitzero"`
	Columns []PluginTerminalColumn `json:"columns"`
}

type PluginTerminalPaging struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

type PluginTerminalColumn struct {
	Title string `json:"title"`
	Field string `json:"field"`
}

type PluginTerminalAction struct {
	ID      string                         `json:"id"`
	Title   string                         `json:"title"`
	Method  string                         `json:"method"`
	Target  extension.TerminalActionTarget `json:"target"`
	Confirm bool                           `json:"confirm"`
	Input   []PluginTerminalInput          `json:"input"`
}

type PluginTerminalInput struct {
	Path  string                `json:"path"`
	Value PluginTerminalBinding `json:"value"`
}

type PluginTerminalBinding struct {
	Source  extension.TerminalBindingSource `json:"source"`
	Value   string                          `json:"value,omitempty"`
	Field   string                          `json:"field,omitempty"`
	Context extension.TerminalContext       `json:"context,omitempty"`
	Form    *PluginTerminalFormField        `json:"form,omitzero"`
}

type PluginTerminalFormField struct {
	Title   string                     `json:"title"`
	Kind    extension.TerminalFormKind `json:"kind"`
	Options []string                   `json:"options,omitempty"`
	Default *PluginTerminalBinding     `json:"default,omitzero"`
}

type ReadPluginResourceRequest struct {
	PluginReleaseRequest
	ComponentID string `json:"componentId"`
	Path        string `json:"path"`
}

type PluginResource struct {
	Body      string `json:"body"`
	MediaType string `json:"mediaType"`
}

type PluginChangeNotice struct {
	Contribution string `json:"contribution"`
}
