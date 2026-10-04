package protocol

// PluginInstallation projects durable admission independently of component readiness.
type PluginInstallation struct {
	Availability    []PluginDiagnostic   `json:"availability"`
	Grants          []PluginRequestGrant `json:"grants"`
	ID              string               `json:"id"`
	Source          string               `json:"source"`
	Selected        PluginRelease        `json:"selected"`
	Staged          *PluginRelease       `json:"staged,omitzero"`
	Enabled         bool                 `json:"enabled"`
	ApprovedDigest  string               `json:"approvedDigest,omitempty"`
	Values          map[string]string    `json:"values"`
	DisabledServers []string             `json:"disabledServers"`
	DisabledSkills  []string             `json:"disabledSkills"`
}
type PluginRelease struct {
	Requests    []PluginRequestGrant      `json:"requests"`
	Digest      string                    `json:"digest"`
	Name        string                    `json:"name"`
	Version     string                    `json:"version,omitempty"`
	Description string                    `json:"description,omitempty"`
	Servers     []PluginServerDeclaration `json:"servers"`
	Inputs      []PluginInput             `json:"inputs"`
	Themes      []PluginTheme             `json:"themes"`
	Skills      []PluginSkill             `json:"skills"`
	Diagnostics []PluginDiagnostic        `json:"diagnostics"`
}
type PluginServerDeclaration struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	CWD     string            `json:"cwd,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}
type PluginInput struct {
	ID       string `json:"id"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
	Server   string `json:"server"`
	Target   string `json:"target"`
	Key      string `json:"key,omitempty"`
}
type PluginRequestGrant struct {
	Capability string   `json:"capability"`
	Targets    []string `json:"targets"`
}

type PluginTheme struct {
	ID     string            `json:"id"`
	Title  string            `json:"title"`
	Scheme PluginThemeScheme `json:"scheme"`
	Colors map[string]string `json:"colors"`
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
type PluginDiagnostic struct {
	Component string `json:"component"`
	Code      string `json:"code"`
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
	InstallationID  string                       `json:"installationId"`
	Digest          string                       `json:"digest"`
	ValueChanges    map[string]PluginValueChange `json:"valueChanges"`
	DisabledServers []string                     `json:"disabledServers"`
	DisabledSkills  []string                     `json:"disabledSkills"`
}
type ApprovePluginRequest struct {
	InstallationID string               `json:"installationId"`
	Digest         string               `json:"digest"`
	Grants         []PluginRequestGrant `json:"grants"`
}

type PluginValueChangeType string

const (
	PluginValueSet   PluginValueChangeType = "set"
	PluginValueClear PluginValueChangeType = "clear"
)

type PluginValueChange struct {
	Type  PluginValueChangeType `json:"type"`
	Value *string               `json:"value,omitzero"`
}

type PluginRemoval struct {
	Availability []PluginDiagnostic `json:"availability"`
}
