package protocol

//flame:enum
type AgentDocScope string

const (
	AgentDocScopeCWD         AgentDocScope = "cwd"
	AgentDocScopeProjectRoot AgentDocScope = "projectRoot"
	AgentDocScopeHome        AgentDocScope = "home"
)

type AgentDoc struct {
	Path  string        `json:"path"`
	Scope AgentDocScope `json:"scope"`
}
