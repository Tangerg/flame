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
	definition func(context.Context, mcpserver.ID) (mcpserver.Server, bool, error)
	a2a        map[string]fingerprint.Digest
}

func NewAuthorities(definition func(context.Context, mcpserver.ID) (mcpserver.Server, bool, error), agents []A2AAgentConfig) *Authorities {
	a := &Authorities{definition: definition, a2a: make(map[string]fingerprint.Digest)}
	for _, agent := range agents {
		a.a2a[agent.Name] = agent.AuthorityFingerprint()
	}
	return a
}
func (a *Authorities) Fingerprint(ctx context.Context, ref tool.Ref) (fingerprint.Digest, bool, error) {
	if err := ref.Validate(); err != nil {
		return fingerprint.Digest{}, false, err
	}
	switch ref.Kind() {
	case tool.BuiltInKind:
		return fingerprint.Digest{}, true, nil
	case tool.MCPKind:
		if a.definition == nil {
			return fingerprint.Digest{}, false, fmt.Errorf("toolset: MCP definition lookup is required")
		}
		id, _, _ := ref.MCP()
		server, found, err := a.definition(ctx, id)
		if err != nil || !found {
			return fingerprint.Digest{}, found, err
		}
		return server.AuthorityFingerprint(), true, nil
	case tool.A2AKind:
		endpoint, _ := ref.A2A()
		authority, found := a.a2a[endpoint]
		return authority, found, nil
	default:
		return fingerprint.Digest{}, false, fmt.Errorf("toolset: unsupported source")
	}
}

// Fingerprints projects each MCP source once within this read. The result has
// no authority to admit dispatch and is never retained across requests.
func (a *Authorities) Fingerprints(ctx context.Context, refs []tool.Ref) (map[tool.Ref]fingerprint.Digest, error) {
	type observedSource struct {
		authority fingerprint.Digest
		found     bool
	}
	sources := make(map[mcpserver.ID]observedSource)
	result := make(map[tool.Ref]fingerprint.Digest, len(refs))
	for _, ref := range refs {
		server, _, isMCP := ref.MCP()
		if isMCP {
			if observed, found := sources[server]; found {
				if observed.found {
					result[ref] = observed.authority
				}
				continue
			}
		}
		authority, found, err := a.Fingerprint(ctx, ref)
		if err != nil {
			return nil, err
		}
		if isMCP {
			sources[server] = observedSource{authority: authority, found: found}
		}
		if found {
			result[ref] = authority
		}
	}
	return result, nil
}
func (a A2AAgentConfig) AuthorityFingerprint() fingerprint.Digest {
	origins := slices.Clone(a.AllowedRPCOrigins)
	slices.Sort(origins)
	origins = slices.Compact(origins)
	return fingerprint.Strings(append([]string{a.CardURL}, origins...)...)
}
