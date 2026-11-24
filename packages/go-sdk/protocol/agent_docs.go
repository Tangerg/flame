package protocol

//flame:enum
type AgentDocScope string

const (
	AgentDocScopeCWD            AgentDocScope = "cwd"
	AgentDocScopeRepositoryRoot AgentDocScope = "repositoryRoot"
	AgentDocScopeHome           AgentDocScope = "home"
)

type AgentDoc struct {
	Path  string        `json:"path"`
	Scope AgentDocScope `json:"scope"`
}
