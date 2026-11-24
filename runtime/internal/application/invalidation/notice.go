// A notice carries IDs only: a value would be a second source of truth that
// disagrees once a notice is coalesced or dropped, which this channel may do.
package invalidation

import (
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

type Resource string

const (
	Resync     Resource = "resync"
	Plugins    Resource = "plugins"
	Sessions   Resource = "sessions"
	Runs       Resource = "runs"
	Interrupts Resource = "interrupts"
	Goals      Resource = "goals"
	Plan       Resource = "plan"
	PluginData Resource = "pluginData"
	Hooks      Resource = "hooks"
	Skills     Resource = "skills"
	MCP        Resource = "mcp"
	Models     Resource = "models"
	Approvals  Resource = "approvals"
	Projects   Resource = "projects"
)

func (r Resource) Valid() bool {
	return r == Plugins || r == Resync || r == Sessions || r == Runs || r == Interrupts ||
		r == Goals || r == Plan || r == PluginData ||
		r == Hooks || r == Skills || r == MCP || r == Models ||
		r == Approvals || r == Projects
}

func (r Resource) String() string {
	if !r.Valid() {
		return "invalid"
	}
	return string(r)
}

type Notice struct {
	Resource   Resource
	SessionIDs []identity.SessionID
	RunIDs     []identity.RunID
	Servers    []mcpserver.ID
}

func (n Notice) Clone() Notice {
	n.SessionIDs = slices.Clone(n.SessionIDs)
	n.RunIDs = slices.Clone(n.RunIDs)
	n.Servers = slices.Clone(n.Servers)
	return n
}

func InSession(resource Resource, sessionID identity.SessionID, runIDs ...identity.RunID) Notice {
	return Notice{Resource: resource, SessionIDs: sessionIDs(sessionID), RunIDs: slices.Clone(runIDs)}
}

func InSessions(resource Resource, ids ...identity.SessionID) Notice {
	return Notice{Resource: resource, SessionIDs: slices.Clone(ids)}
}

func ForMCP(servers ...mcpserver.ID) Notice {
	return Notice{Resource: MCP, Servers: slices.Clone(servers)}
}

func sessionIDs(id identity.SessionID) []identity.SessionID {
	if id.IsZero() {
		return nil
	}
	return []identity.SessionID{id}
}

type Publish func(Notice)

func (p Publish) Notify(notices ...Notice) {
	for _, notice := range notices {
		p(notice.Clone())
	}
}
