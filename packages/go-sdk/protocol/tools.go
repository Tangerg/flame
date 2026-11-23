package protocol

type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	SafetyClass SafetyClass    `json:"safetyClass,omitempty"`
}

type InvokeToolRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Workspace *WorkspaceRef  `json:"workspace,omitzero"`
}
