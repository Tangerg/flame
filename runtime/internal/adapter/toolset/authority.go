package toolset

import (
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

// Authorities projects permission fingerprints from current source owners.
// Connection dispatch separately checks the configuration its executable realized.
type Authorities struct {
	definition func(context.Context, mcpserver.ServerName) (mcpserver.Server, bool, error)
	a2a        map[string]string
}

func NewAuthorities(definition func(context.Context, mcpserver.ServerName) (mcpserver.Server, bool, error), agents []A2AAgentConfig) *Authorities {
	a := &Authorities{definition: definition, a2a: make(map[string]string)}
	for _, agent := range agents {
		a.a2a[agent.Name] = agent.AuthorityFingerprint()
	}
	return a
}
func (a *Authorities) Fingerprint(ctx context.Context, ref tool.Ref) (string, bool, error) {
	if err := ref.Validate(); err != nil {
		return "", false, err
	}
	switch ref.Kind() {
	case tool.BuiltInKind:
		return "", true, nil
	case tool.MCPKind:
		if a.definition == nil {
			return "", false, fmt.Errorf("toolset: MCP definition lookup is required")
		}
		server, found, err := a.definition(ctx, ref.Server())
		if err != nil || !found {
			return "", found, err
		}
		return server.AuthorityFingerprint(), true, nil
	case tool.A2AKind:
		fingerprint, found := a.a2a[ref.Name()]
		return fingerprint, found, nil
	default:
		return "", false, fmt.Errorf("toolset: unsupported source")
	}
}

// Fingerprints projects each MCP source once within this read. The result has
// no authority to admit dispatch and is never retained across requests.
func (a *Authorities) Fingerprints(ctx context.Context, refs []tool.Ref) (map[tool.Ref]string, error) {
	type observedSource struct {
		fingerprint string
		found       bool
	}
	sources := make(map[mcpserver.ServerName]observedSource)
	result := make(map[tool.Ref]string, len(refs))
	for _, ref := range refs {
		if ref.Kind() == tool.MCPKind {
			if observed, found := sources[ref.Server()]; found {
				if observed.found {
					result[ref] = observed.fingerprint
				}
				continue
			}
		}
		fingerprint, found, err := a.Fingerprint(ctx, ref)
		if err != nil {
			return nil, err
		}
		if ref.Kind() == tool.MCPKind {
			sources[ref.Server()] = observedSource{fingerprint: fingerprint, found: found}
		}
		if found {
			result[ref] = fingerprint
		}
	}
	return result, nil
}
func (a A2AAgentConfig) AuthorityFingerprint() string {
	origins := slices.Clone(a.AllowedRPCOrigins)
	slices.Sort(origins)
	origins = slices.Compact(origins)
	return fingerprint.Strings(append([]string{a.CardURL}, origins...)...)
}
