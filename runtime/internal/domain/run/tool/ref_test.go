package tool

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

const testInstallation = "8ad9abf5-3a7d-4d0b-bef9-6ef92c20e746"

func testMCPRef(t *testing.T, origin mcpserver.Origin, server, remote string) Ref {
	t.Helper()
	name, err := mcpserver.ParseServerName(server)
	if err != nil {
		t.Fatal(err)
	}
	id, err := mcpserver.NewID(origin, name)
	if err != nil {
		t.Fatal(err)
	}
	n, err := mcpserver.ParseRemoteToolName(remote)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := MCP(id, n)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func testInstallationOrigin(t *testing.T) mcpserver.Origin {
	t.Helper()
	id, err := resourceid.ParseInstallation(testInstallation)
	if err != nil {
		t.Fatal(err)
	}
	origin, err := mcpserver.InstallationOrigin(id)
	if err != nil {
		t.Fatal(err)
	}
	return origin
}

func TestParseRefPreservesIdentityFailureCategoryAndCause(t *testing.T) {
	_, err := ParseRef("mcp:user:Invalid:read")
	if !errors.Is(err, ErrInvalidRef) || !errors.Is(err, mcpserver.ErrInvalidServerName) {
		t.Fatalf("invalid server reference = %v, want reference and server categories", err)
	}
	_, err = ParseRef("mcp:user:files:read file")
	if !errors.Is(err, ErrInvalidRef) || !errors.Is(err, mcpserver.ErrInvalidRemoteToolName) {
		t.Fatalf("invalid remote reference = %v, want reference and remote categories", err)
	}
}

func TestRefPreservesSourceAcrossModelNameCollisions(t *testing.T) {
	user := mcpserver.UserOrigin()
	installed := testInstallationOrigin(t)
	for _, pair := range [][2]Ref{
		{testMCPRef(t, user, "a_b", "c"), testMCPRef(t, user, "a", "b_c")},
		{testMCPRef(t, user, "files", "read.file"), testMCPRef(t, user, "files", "read_file")},
		{testMCPRef(t, user, "files", strings.Repeat("a", 70)), testMCPRef(t, user, "files", strings.Repeat("a", 70)+"b")},
		{testMCPRef(t, user, "files", "read"), testMCPRef(t, installed, "files", "read")},
	} {
		if pair[0] == pair[1] || pair[0].ModelName() != pair[1].ModelName() {
			t.Fatalf("identity/projection = %v", pair)
		}
		for _, ref := range pair {
			parsed, err := ParseRef(ref.String())
			if err != nil || parsed != ref {
				t.Fatalf("roundtrip %s: %v", ref, err)
			}
		}
	}
	if _, err := BuiltIn("remote"); err == nil {
		t.Fatal("unknown built-in accepted")
	}
	if (Ref{}).Validate() == nil {
		t.Fatal("zero reference accepted")
	}
	for _, raw := range []string{
		"mcp:user:files:read:extra", "mcp:files:read", "mcp:installation:files:read",
		"mcp:installation:00000000-0000-0000-0000-000000000000:files:read", "mcp:user:installation:" + testInstallation + ":read",
		"builtIn:%73hell", "shell", "a2a:with space", "builtIn:shell:extra",
	} {
		if _, err := ParseRef(raw); err == nil {
			t.Errorf("accepted noncanonical reference %q", raw)
		}
	}
}

func TestRefVariantsAreReachableOnlyThroughTheirKind(t *testing.T) {
	builtIn, err := BuiltIn(Shell)
	if err != nil {
		t.Fatal(err)
	}
	a2a, err := A2A("planner")
	if err != nil {
		t.Fatal(err)
	}
	mcp := testMCPRef(t, testInstallationOrigin(t), "files", "read")
	refs := map[RefKind]Ref{BuiltInKind: builtIn, MCPKind: mcp, A2AKind: a2a}
	kinds := make([]RefKind, 0, len(refs))
	for kind := range refs {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	expected := RefKinds()
	slices.Sort(expected)
	if !slices.Equal(kinds, expected) {
		t.Fatalf("test covers %v, closed set is %v", kinds, expected)
	}
	for kind, ref := range refs {
		_, isBuiltIn := ref.BuiltIn()
		_, _, isMCP := ref.MCP()
		_, isA2A := ref.A2A()
		if ref.Kind() != kind || isBuiltIn != (kind == BuiltInKind) || isMCP != (kind == MCPKind) || isA2A != (kind == A2AKind) {
			t.Errorf("%s accessors = builtIn %t, mcp %t, a2a %t", kind, isBuiltIn, isMCP, isA2A)
		}
		parsed, err := ParseRef(ref.String())
		if err != nil || parsed != ref {
			t.Errorf("%s text roundtrip = %v, %v", kind, parsed, err)
		}
	}
	if server, remote, ok := mcp.MCP(); !ok || server.Origin().Kind() != mcpserver.OriginInstallation || remote.String() != "read" {
		t.Fatalf("MCP variant = %v, %v, %v", server, remote, ok)
	}
	var zero Ref
	if _, ok := zero.BuiltIn(); ok {
		t.Fatal("zero reference reported a variant")
	}
}

func TestRefFingerprintFollowsSourceKind(t *testing.T) {
	builtIn, err := BuiltIn(Shell)
	if err != nil {
		t.Fatal(err)
	}
	mcp := testMCPRef(t, mcpserver.UserOrigin(), "files", "read")
	authority := fingerprint.Strings("endpoint")
	if builtIn.ValidateFingerprint(fingerprint.Digest{}) != nil || builtIn.ValidateFingerprint(authority) == nil {
		t.Fatal("built-in fingerprint rule")
	}
	if mcp.ValidateFingerprint(authority) != nil || mcp.ValidateFingerprint(fingerprint.Digest{}) == nil {
		t.Fatal("MCP fingerprint rule")
	}
}
