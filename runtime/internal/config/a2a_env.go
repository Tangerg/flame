package config

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/httporigin"
)

// parseA2AAgents parses the FLAME_A2A_AGENTS env var: a comma-separated list of
// "name=cardURL" pairs, where cardURL is the base URL the remote agent's
// AgentCard is resolved from. Empty input yields nil. The name becomes the
// delegation tool's name; the first '=' separates it from the URL, so query
// strings in the URL are preserved.
func parseA2AAgents(raw string) ([]A2AAgent, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]A2AAgent, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for index, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		eq := strings.IndexByte(p, '=')
		if eq <= 0 || eq == len(p)-1 {
			return nil, fmt.Errorf("entry %d: expected name=cardURL", index+1)
		}
		name := strings.TrimSpace(p[:eq])
		cardURL := strings.TrimSpace(p[eq+1:])
		if name == "" || cardURL == "" {
			return nil, fmt.Errorf("entry %d: name and cardURL must be non-empty", index+1)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("agent %q is configured more than once", name)
		}
		if _, err := httporigin.Parse(cardURL); err != nil {
			return nil, fmt.Errorf("agent %q: cardURL must be a valid HTTP(S) URL without credentials", name)
		}
		seen[name] = struct{}{}
		out = append(out, A2AAgent{Name: name, CardURL: cardURL})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// addA2ARPCOrigins applies the optional FLAME_A2A_RPC_ORIGINS map to parsed
// agents. The shape is "name=origin|origin,name=origin"; names must already
// exist in FLAME_A2A_AGENTS so a misspelling cannot silently weaken nothing.
func addA2ARPCOrigins(agents []A2AAgent, raw string) ([]A2AAgent, error) {
	if raw == "" {
		return agents, nil
	}
	out := make([]A2AAgent, len(agents))
	for i, agent := range agents {
		out[i] = agent
		out[i].AllowedRPCOrigins = slices.Clone(agent.AllowedRPCOrigins)
	}
	byName := make(map[string]int, len(agents))
	for i, agent := range out {
		if _, duplicate := byName[agent.Name]; duplicate {
			return nil, fmt.Errorf("agent %q is configured more than once", agent.Name)
		}
		byName[agent.Name] = i
	}
	configured := make(map[string]struct{})
	for entry := range strings.SplitSeq(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, rawOrigins, ok := strings.Cut(entry, "=")
		name, rawOrigins = strings.TrimSpace(name), strings.TrimSpace(rawOrigins)
		if !ok || name == "" || rawOrigins == "" {
			return nil, fmt.Errorf("entry %q: expected name=origin|origin", entry)
		}
		index, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("entry %q: agent %q is not configured", entry, name)
		}
		if _, duplicate := configured[name]; duplicate {
			return nil, fmt.Errorf("entry %q: agent %q is configured more than once", entry, name)
		}
		origins := strings.Split(rawOrigins, "|")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
			if origins[i] == "" {
				return nil, fmt.Errorf("entry %q: RPC origins must not be empty", entry)
			}
		}
		out[index].AllowedRPCOrigins = origins
		configured[name] = struct{}{}
	}
	return out, nil
}
