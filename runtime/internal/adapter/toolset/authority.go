package toolset

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// Authorities projects permission fingerprints from current source owners.
// Connection dispatch separately checks the configuration its executable realized.
type Authorities struct {
	definition func(context.Context, mcpserver.ID) (mcpserver.Server, bool, error)
}

func NewAuthorities(definition func(context.Context, mcpserver.ID) (mcpserver.Server, bool, error)) *Authorities {
	return &Authorities{definition: definition}
}
func (a *Authorities) Fingerprint(ctx context.Context, ref tool.Ref) (identity.Digest, bool, error) {
	if err := ref.Validate(); err != nil {
		return identity.Digest{}, false, err
	}
	switch ref.Kind() {
	case tool.BuiltInKind:
		return identity.Digest{}, true, nil
	case tool.MCPKind:
		if a.definition == nil {
			return identity.Digest{}, false, fmt.Errorf("toolset: MCP definition lookup is required")
		}
		id, _, _ := ref.MCP()
		server, found, err := a.definition(ctx, id)
		if err != nil || !found {
			return identity.Digest{}, found, err
		}
		return server.AuthorityFingerprint(), true, nil
	default:
		return identity.Digest{}, false, fmt.Errorf("toolset: unsupported source")
	}
}

// Fingerprints projects each MCP source once within this read. The result has
// no authority to admit dispatch and is never retained across requests.
func (a *Authorities) Fingerprints(ctx context.Context, refs []tool.Ref) (map[tool.Ref]identity.Digest, error) {
	type observedSource struct {
		authority identity.Digest
		found     bool
	}
	sources := make(map[mcpserver.ID]observedSource)
	result := make(map[tool.Ref]identity.Digest, len(refs))
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
