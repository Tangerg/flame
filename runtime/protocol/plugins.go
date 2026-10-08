package protocol

// PluginInstallation projects durable admission beside the realization the
// Runtime observes for it now.
type PluginInstallation struct {
	Realization     PluginRealization           `json:"realization"`
	Presentation    PluginPresentation          `json:"presentation"`
	ID              string                      `json:"id"`
	Source          string                      `json:"source"`
	Selected        PluginRelease               `json:"selected"`
	Staged          *PluginRelease              `json:"staged,omitzero"`
	State           PluginInstallationState     `json:"state"`
	InputStates     map[string]PluginInputState `json:"inputStates"`
	DisabledServers []string                    `json:"disabledServers"`
	DisabledSkills  []string                    `json:"disabledSkills"`
}

// PluginInstallationState is the closed trust state of an installation.
// Approval names the selected release: selecting another release returns the
// installation to unapproved.
type PluginInstallationState string

const (
	PluginInstallationUnapproved PluginInstallationState = "unapproved"
	PluginInstallationApproved   PluginInstallationState = "approved"
	PluginInstallationEnabled    PluginInstallationState = "enabled"
)

// PluginPresentation is the Runtime's decision whether the selected release's
// declarative presentation contributions, such as themes, are admitted now.
// Clients present exactly the admitted ones and never derive it themselves.
type PluginPresentation string

const (
	PluginPresentationAdmitted PluginPresentation = "admitted"
	PluginPresentationWithheld PluginPresentation = "withheld"
)

// PluginRealizationType is a closed union. It is observed from the selected
// release bytes and backend directories on every read and is never stored.
type PluginRealizationType string

const (
	PluginRealizationAvailable          PluginRealizationType = "available"
	PluginRealizationReleaseUnavailable PluginRealizationType = "releaseUnavailable"
)

// PluginRealization lists, only when the release is available, the declared
// servers whose backend cannot be realized now; empty means every backend is.
type PluginRealization struct {
	Type                PluginRealizationType `json:"type"`
	UnavailableBackends []string              `json:"unavailableBackends,omitzero"`
}

// PluginInputStateType is a closed union over every declared input of the
// selected release. A configured secret is reported as configured; its value
// never leaves the Runtime.
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
	Digest      string                    `json:"digest"`
	Name        string                    `json:"name"`
	Version     string                    `json:"version,omitempty"`
	Description string                    `json:"description,omitempty"`
	Servers     []PluginServerDeclaration `json:"servers"`
	Inputs      []PluginInput             `json:"inputs"`
	Themes      []PluginTheme             `json:"themes"`
	Views       []PluginView              `json:"views"`
	Skills      []PluginSkill             `json:"skills"`
	Diagnostics []PluginDiagnostic        `json:"diagnostics"`
}

type PluginViewType string

const PluginViewSessionTrajectory PluginViewType = "sessionTrajectory"

type PluginView struct {
	ID    string         `json:"id"`
	Title string         `json:"title"`
	Type  PluginViewType `json:"type"`
}

// ReadPluginViewRequest addresses an admitted resource, never a filesystem path.
type ReadPluginViewRequest struct {
	PluginReleaseRequest
	ViewID string `json:"viewId"`
}

type PluginViewResource struct {
	HTML string `json:"html"`
}

// ReadPluginTrajectoryRequest scopes the existing trajectory projection to
// the Session selected by the trusted host. It grants no execution operations.
type ReadPluginTrajectoryRequest struct {
	ReadPluginViewRequest
	SessionID string `json:"sessionId"`
	PageQuery
}

// PluginServerDeclaration is a closed union by transport that projects a
// release's server declaration in the MCP vocabulary. stdio carries command,
// args, env and dir; streamableHttp carries url and headers. Command and Dir
// keep their package-relative spelling.
type PluginServerDeclaration struct {
	Name    string            `json:"name"`
	Type    MCPTransport      `json:"type"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Dir     string            `json:"dir,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// PluginInput is one value the user supplies for a declared server. env and
// header inputs name their key; authorization carries none.
type PluginInput struct {
	ID       string            `json:"id"`
	Secret   bool              `json:"secret"`
	Required bool              `json:"required"`
	Server   string            `json:"server"`
	Target   PluginInputTarget `json:"target"`
	Key      string            `json:"key,omitempty"`
}

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

// PluginThemeColors is the closed set of colors a theme may override; an
// absent color is not overridden.
type PluginThemeColors struct {
	Background string `json:"background,omitempty"`
	Foreground string `json:"foreground,omitempty"`
	Accent     string `json:"accent,omitempty"`
	Muted      string `json:"muted,omitempty"`
	Border     string `json:"border,omitempty"`
}

type PluginThemeScheme string

const (
	PluginThemeDark  PluginThemeScheme = "dark"
	PluginThemeLight PluginThemeScheme = "light"
)

type PluginSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PluginDiagnostic is one admission finding about the release bytes.
type PluginDiagnostic struct {
	Component PluginComponent      `json:"component"`
	Code      PluginDiagnosticCode `json:"code"`
}

type PluginDiagnosticCode string

const (
	PluginDiagnosticUnknownField            PluginDiagnosticCode = "unknownField"
	PluginDiagnosticInvalidDeclaration      PluginDiagnosticCode = "invalidDeclaration"
	PluginDiagnosticUnsupportedContribution PluginDiagnosticCode = "unsupportedContribution"
	PluginDiagnosticComponentLimit          PluginDiagnosticCode = "componentLimit"
	PluginDiagnosticInvalidDependencies     PluginDiagnosticCode = "invalidDependencies"
	PluginDiagnosticUnavailableComponent    PluginDiagnosticCode = "unavailableComponent"
)

// PluginComponentType is a closed union. Kinds that address an authored
// member carry its exact authored name; the others carry none.
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

// ConfigurePluginRequest is a delta: inputs and components it does not name
// keep their current value or enablement.
type ConfigurePluginRequest struct {
	InstallationID string                           `json:"installationId"`
	Digest         string                           `json:"digest"`
	ValueChanges   map[string]PluginValueChange     `json:"valueChanges"`
	ServerChanges  map[string]PluginComponentChange `json:"serverChanges"`
	SkillChanges   map[string]PluginComponentChange `json:"skillChanges"`
}

type PluginComponentChange string

const (
	PluginComponentEnable  PluginComponentChange = "enable"
	PluginComponentDisable PluginComponentChange = "disable"
)

type PluginValueChangeType string

const (
	PluginValueSet   PluginValueChangeType = "set"
	PluginValueClear PluginValueChangeType = "clear"
)

type PluginValueChange struct {
	Type  PluginValueChangeType `json:"type"`
	Value *string               `json:"value,omitzero"`
}
