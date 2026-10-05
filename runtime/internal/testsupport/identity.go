package testsupport

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/google/uuid"
)

func BuiltInTool(t testing.TB, name tool.BuiltInName) tool.Ref {
	t.Helper()
	ref, err := tool.BuiltIn(name)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func A2ATool(t testing.TB, endpoint string) tool.Ref {
	t.Helper()
	ref, err := tool.A2A(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func ToolFingerprint(ref tool.Ref) fingerprint.Digest {
	if ref.Kind() == tool.BuiltInKind {
		return fingerprint.Digest{}
	}
	return Digest("source authority")
}

// Digest derives a distinct valid digest from seed.
func Digest(seed string) fingerprint.Digest { return fingerprint.Strings(seed) }

// Release admits declaration as the release whose digest derives from seed.
func Release(t testing.TB, seed string, declaration plugin.Declaration) plugin.Release {
	t.Helper()
	release, err := plugin.NewRelease(Digest(seed), declaration)
	if err != nil {
		t.Fatal(err)
	}
	return release
}

func InstallationID(t testing.TB) resourceid.InstallationID {
	t.Helper()
	id, err := resourceid.ParseInstallation(uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// The MCP identity fixtures below panic on an invalid literal instead of
// taking a testing.TB, so tables declared outside a test can use them too.

func UserMCPServer(name string) mcpserver.ID {
	return must(mcpserver.NewID(mcpserver.UserOrigin(), ServerName(name)))
}

func InstallationMCPServer(installation resourceid.InstallationID, name string) mcpserver.ID {
	return must(mcpserver.NewID(must(mcpserver.InstallationOrigin(installation)), ServerName(name)))
}

func MCPTool(server mcpserver.ID, remote string) tool.Ref {
	return must(tool.MCP(server, RemoteToolName(remote)))
}

func ServerName(raw string) mcpserver.ServerName {
	return must(mcpserver.ParseServerName(raw))
}

func RemoteToolName(raw string) mcpserver.RemoteToolName {
	return must(mcpserver.ParseRemoteToolName(raw))
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

type ToolAuthorities struct{}

func (ToolAuthorities) Fingerprint(_ context.Context, ref tool.Ref) (fingerprint.Digest, bool, error) {
	if err := ref.Validate(); err != nil {
		return fingerprint.Digest{}, false, err
	}
	return ToolFingerprint(ref), true, nil
}

func (a ToolAuthorities) Fingerprints(ctx context.Context, refs []tool.Ref) (map[tool.Ref]fingerprint.Digest, error) {
	result := make(map[tool.Ref]fingerprint.Digest, len(refs))
	for _, ref := range refs {
		fingerprint, found, err := a.Fingerprint(ctx, ref)
		if err != nil {
			return nil, err
		}
		if found {
			result[ref] = fingerprint
		}
	}
	return result, nil
}

const (
	BuildID                       = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	AlternateBuildID              = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	RuntimeInstanceID             = "runtime_11111111-1111-1111-1111-111111111111"
	AlternateRuntimeInstanceID    = "runtime_22222222-2222-2222-2222-222222222222"
	IdempotencyNamespace          = "idp_11111111111111111111111111111111"
	AlternateIdempotencyNamespace = "idp_22222222222222222222222222222222"
	MCPAuthorizationAttemptID     = "mcpauth_AAAAAAAAAAAAAAAAAAAAAAAAAA"
)
