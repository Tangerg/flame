package mcpserver

// ToolPolicy is the effective per-tool policy derived from enabled MCP server
// registrations. It is immutable after construction and safe for concurrent
// readers.
type ToolPolicy struct {
	enabled      map[ServerName]struct{}
	disabled     map[ToolRef]struct{}
	autoApproved map[ToolRef]struct{}
}

// NewToolPolicy derives the effective tool policy from enabled servers.
func NewToolPolicy(servers []Server) ToolPolicy {
	var policy ToolPolicy
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		if policy.enabled == nil {
			policy.enabled = map[ServerName]struct{}{}
		}
		policy.enabled[server.Name] = struct{}{}
		for _, rule := range server.ToolPolicy.Rules() {
			ref := ToolRef{Server: server.Name, Tool: rule.Tool}
			switch rule.Decision {
			case ToolDisabled:
				if policy.disabled == nil {
					policy.disabled = map[ToolRef]struct{}{}
				}
				policy.disabled[ref] = struct{}{}
			case ToolAutoApproved:
				if policy.autoApproved == nil {
					policy.autoApproved = map[ToolRef]struct{}{}
				}
				policy.autoApproved[ref] = struct{}{}
			}
		}
	}
	return policy
}

// Disabled reports whether ref is hidden from resolution.
func (t ToolPolicy) Disabled(ref ToolRef) bool {
	if _, ok := t.enabled[ref.Server]; !ok {
		return true
	}
	_, ok := t.disabled[ref]
	return ok
}

// AutoApproved reports whether ref may skip the interactive
// approval prompt after standing approval rules have been evaluated.
func (t ToolPolicy) AutoApproved(ref ToolRef) bool {
	_, ok := t.autoApproved[ref]
	return ok
}
