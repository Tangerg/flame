package testsupport

import (
	"context"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"testing"
)

func BuiltInTool(t testing.TB, name string) tool.Ref {
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

func ToolFingerprint(ref tool.Ref) string {
	if ref.Kind() == tool.BuiltInKind {
		return ""
	}
	return "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
}

type ToolAuthorities struct{}

func (ToolAuthorities) Fingerprint(_ context.Context, ref tool.Ref) (string, bool, error) {
	if err := ref.Validate(); err != nil {
		return "", false, err
	}
	return ToolFingerprint(ref), true, nil
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
