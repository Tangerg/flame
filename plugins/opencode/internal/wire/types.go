package wire

const (
	List       = "flame.opencode/list.v1"
	Open       = "flame.opencode/open.v1"
	Get        = "flame.opencode/get.v1"
	Prompt     = "flame.opencode/prompt.v1"
	Cancel     = "flame.opencode/cancel.v1"
	Permission = "flame.opencode/permission.v1"
	Fork       = "flame.opencode/fork.v1"
)

type ListRequest struct {
	CWD    string `json:"cwd"`
	Cursor string `json:"cursor,omitempty"`
}

type NativeSession struct {
	ID    string `json:"id"`
	CWD   string `json:"cwd"`
	Title string `json:"title,omitempty"`
}

type Sessions struct {
	Machine    string          `json:"machine"`
	Agent      string          `json:"agent"`
	Rows       []NativeSession `json:"rows"`
	NextCursor string          `json:"nextCursor,omitempty"`
}

type OpenRequest struct {
	NativeID string `json:"nativeId"`
	CWD      string `json:"cwd"`
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
	ID          string              `json:"id"`
	NativeID    string              `json:"nativeId"`
	CWD         string              `json:"cwd"`
	Machine     string              `json:"machine"`
	Agent       string              `json:"agent"`
	Connected   bool                `json:"connected"`
	CanFork     bool                `json:"canFork"`
	Events      []Event             `json:"events"`
	Permissions []PermissionRequest `json:"permissions"`
	Turn        *Turn               `json:"turn,omitzero"`
}
