package wire

type Agent struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Agents struct {
	Agents []Agent `json:"agents"`
}

type SendRequest struct {
	Agent   string `json:"agent"`
	Message string `json:"message"`
}

type TaskRequest struct {
	RequestID string `json:"requestId"`
}

type Result struct {
	RequestID string `json:"requestId"`
	Agent     string `json:"agent"`
	Kind      string `json:"kind" jsonschema:"enum=task,enum=message"`
	TaskID    string `json:"taskId,omitempty"`
	ContextID string `json:"contextId,omitempty"`
	State     string `json:"state,omitempty"`
	ValueJSON string `json:"valueJSON"`
}
