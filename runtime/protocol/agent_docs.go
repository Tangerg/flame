package protocol

// AgentDocScope is where an AGENTS.md was discovered in the cwd→home hierarchy.
type AgentDocScope string

const (
	AgentDocScopeCWD         AgentDocScope = "cwd"
	AgentDocScopeProjectRoot AgentDocScope = "projectRoot"
	AgentDocScopeHome        AgentDocScope = "home"
)

// AgentDoc is one AGENTS.md in the unique effective cascade. agentDocs.list
// returns documents in prompt render order: home, project-root tree, then cwd.
type AgentDoc struct {
	Path  string        `json:"path"`
	Scope AgentDocScope `json:"scope"` // see AgentDocScope
}
