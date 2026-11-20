package testsupport

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	framevalue "github.com/Tangerg/frame/value"
	"github.com/google/uuid"
)

func ToolFingerprint(ref tool.Ref) framevalue.Digest {
	if ref.Kind() == tool.BuiltInKind {
		return framevalue.Digest{}
	}
	return identity.DigestStrings("source authority")
}

func InstallationID(t testing.TB) identity.InstallationID {
	t.Helper()
	id, err := identity.ParseInstallation(uuid.NewString())
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

const (
	BuildID                       = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	AlternateBuildID              = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	RuntimeInstanceID             = "runtime_11111111-1111-1111-1111-111111111111"
	AlternateRuntimeInstanceID    = "runtime_22222222-2222-2222-2222-222222222222"
	IdempotencyNamespace          = "idp_11111111111111111111111111111111"
	AlternateIdempotencyNamespace = "idp_22222222222222222222222222222222"
	MCPAuthorizationAttemptID     = "mcpauth_AAAAAAAAAAAAAAAAAAAAAAAAAA"
)
