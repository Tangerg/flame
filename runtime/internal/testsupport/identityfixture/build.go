// Package identityfixture provides named, deterministic identities for tests
// that exercise boundaries shared by several Runtime packages.
package identityfixture

const (
	BuildID                       = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	AlternateBuildID              = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	RuntimeInstanceID             = "runtime_11111111-1111-1111-1111-111111111111"
	AlternateRuntimeInstanceID    = "runtime_22222222-2222-2222-2222-222222222222"
	IdempotencyNamespace          = "idp_11111111111111111111111111111111"
	AlternateIdempotencyNamespace = "idp_22222222222222222222222222222222"
	MCPAuthorizationAttemptID     = "mcpauth_AAAAAAAAAAAAAAAAAAAAAAAAAA"
)
