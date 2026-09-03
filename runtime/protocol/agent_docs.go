package protocol

// AgentDocScope is where an AGENTS.md was discovered in the cwd→home hierarchy
// It mirrors KnowledgeScope's values but is a distinct domain (left
// separate rather than DRY-coupled — two scopes is under the rule-of-three).
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
	Title string        `json:"title,omitempty"`
	Scope AgentDocScope `json:"scope"` // see AgentDocScope
}
