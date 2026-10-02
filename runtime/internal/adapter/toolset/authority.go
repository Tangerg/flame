package toolset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

type authorityRegistry interface {
	Get(context.Context, mcpserver.ServerName) (mcpserver.Server, bool, error)
}

// Authorities reads the current source configuration. Executables retain their
// own connection fingerprint, so changing a source cannot authorize an old call.
type Authorities struct {
	registry authorityRegistry
	a2a      map[string]string
}

func NewAuthorities(registry authorityRegistry, agents []A2AAgentConfig) *Authorities {
	a := &Authorities{registry: registry, a2a: make(map[string]string)}
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
		server, found, err := a.registry.Get(ctx, ref.Server())
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
func (a A2AAgentConfig) AuthorityFingerprint() string {
	origins := slices.Clone(a.AllowedRPCOrigins)
	slices.Sort(origins)
	origins = slices.Compact(origins)
	h := sha256.New()
	for _, value := range append([]string{a.CardURL}, origins...) {
		h.Write([]byte(strconv.Itoa(len(value)) + ":" + value))
	}
	return hex.EncodeToString(h.Sum(nil))
}
