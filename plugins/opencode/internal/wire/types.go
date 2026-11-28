package wire

const (
	Agents     = "flame.opencode/agents.v1"
	List       = "flame.opencode/list.v1"
	Create     = "flame.opencode/create.v1"
	Open       = "flame.opencode/open.v1"
	Get        = "flame.opencode/get.v1"
	Prompt     = "flame.opencode/prompt.v1"
	Cancel     = "flame.opencode/cancel.v1"
	Permission = "flame.opencode/permission.v1"
	Fork       = "flame.opencode/fork.v1"
)

type Agent struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Registry string `json:"registry,omitempty"`
	Version  string `json:"version,omitempty"`
}

type AgentsRequest struct{}

type AgentsResponse struct {
	Agents []Agent `json:"agents"`
}

type Capabilities struct {
	List   bool `json:"list"`
	Load   bool `json:"load"`
	Resume bool `json:"resume"`
	Fork   bool `json:"fork"`
}

type ListRequest struct {
	CWD     string `json:"cwd"`
	Cursor  string `json:"cursor,omitempty"`
	AgentID string `json:"agentId,omitempty"`
}

type NativeSession struct {
	ID    string `json:"id"`
	CWD   string `json:"cwd"`
	Title string `json:"title,omitempty"`
}

type Sessions struct {
	Machine      string          `json:"machine"`
	Agent        string          `json:"agent"`
	Capabilities Capabilities    `json:"capabilities"`
	Rows         []NativeSession `json:"rows"`
	NextCursor   string          `json:"nextCursor,omitempty"`
}

type CreateRequest struct {
	CWD     string `json:"cwd"`
	AgentID string `json:"agentId,omitempty"`
}

type OpenRequest struct {
	NativeID string `json:"nativeId,omitempty"`
	CWD      string `json:"cwd,omitempty"`
	AgentID  string `json:"agentId,omitempty"`
	ID       string `json:"id,omitempty"`
	Mode     string `json:"mode,omitempty" jsonschema:"enum=load,enum=resume"`
}

type SessionRequest struct {
	ID string `json:"id"`
}

type TurnRequest struct {
	ID     string `json:"id"`
	TurnID string `json:"turnId"`
}

type PromptRequest struct {
	ID           string `json:"id"`
	PreviousTurn string `json:"previousTurn,omitempty"`
	Continue     bool   `json:"continue,omitzero"`
	Text         string `json:"text"`
}

type AnswerRequest struct {
	ID           string `json:"id"`
	PermissionID string `json:"permissionId"`
	OptionID     string `json:"optionId"`
}

type Turn struct {
	ID              string `json:"id"`
	Status          string `json:"status"`
	StopReason      string `json:"stopReason,omitempty"`
	CancelRequested bool   `json:"cancelRequested"`
}

type Event struct {
	Kind   string `json:"kind"`
	Text   string `json:"text,omitempty"`
	ToolID string `json:"toolId,omitempty"`
	Status string `json:"status,omitempty"`
}

type Option struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type PermissionRequest struct {
	ID       string   `json:"id"`
	TurnID   string   `json:"turnId"`
	ToolID   string   `json:"toolId"`
	Title    string   `json:"title"`
	Preview  string   `json:"preview,omitempty"`
	Options  []Option `json:"options"`
	Status   string   `json:"status"`
	Selected string   `json:"selected,omitempty"`
}

type Session struct {
	ID           string              `json:"id"`
	NativeID     string              `json:"nativeId"`
	CWD          string              `json:"cwd"`
	Machine      string              `json:"machine"`
	Agent        string              `json:"agent"`
	AgentID      string              `json:"agentId,omitempty"`
	Capabilities *Capabilities       `json:"capabilities,omitzero"`
	Connected    bool                `json:"connected"`
	CanFork      bool                `json:"canFork"`
	Events       []Event             `json:"events"`
	Permissions  []PermissionRequest `json:"permissions"`
	Turn         *Turn               `json:"turn,omitzero"`
	Authority    string              `json:"-"`
}
