package toolset

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/qimu/integrity"
)

// Authorities projects permission fingerprints from current source owners.
// Connection dispatch separately checks the configuration its executable realized.
type Authorities struct {
	definition func(context.Context, mcpserver.ID) (mcpserver.Server, bool, error)
}

func NewAuthorities(definition func(context.Context, mcpserver.ID) (mcpserver.Server, bool, error)) *Authorities {
	return &Authorities{definition: definition}
}
func (a *Authorities) Fingerprint(ctx context.Context, ref tool.Ref) (integrity.Digest, bool, error) {
	if err := ref.Validate(); err != nil {
		return integrity.Digest{}, false, err
	}
	switch ref.Kind() {
	case tool.BuiltInKind:
		return integrity.Digest{}, true, nil
	case tool.MCPKind:
		if a.definition == nil {
			return integrity.Digest{}, false, fmt.Errorf("toolset: MCP definition lookup is required")
		}
		id, _, _ := ref.MCP()
		server, found, err := a.definition(ctx, id)
		if err != nil || !found {
			return integrity.Digest{}, found, err
		}
		return server.AuthorityFingerprint(), true, nil
	default:
		return integrity.Digest{}, false, fmt.Errorf("toolset: unsupported source")
	}
}

// Fingerprints projects each MCP source once within this read. The result has
// no authority to admit dispatch and is never retained across requests.
func (a *Authorities) Fingerprints(ctx context.Context, refs []tool.Ref) (map[tool.Ref]integrity.Digest, error) {
	type observedSource struct {
		authority integrity.Digest
		found     bool
	}
	sources := make(map[mcpserver.ID]observedSource)
	result := make(map[tool.Ref]integrity.Digest, len(refs))
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
