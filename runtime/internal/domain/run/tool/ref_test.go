package tool

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

func TestRefPreservesSourceAcrossModelNameCollisions(t *testing.T) {
	mcp := func(server, remote string) Ref {
		s, err := mcpserver.ParseServerName(server)
		if err != nil {
			t.Fatal(err)
		}
		n, err := mcpserver.ParseRemoteToolName(remote)
		if err != nil {
			t.Fatal(err)
		}
		ref, err := MCP(s, n)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	for _, pair := range [][2]Ref{{mcp("a_b", "c"), mcp("a", "b_c")}, {mcp("files", "read.file"), mcp("files", "read_file")}, {mcp("files", strings.Repeat("a", 70)), mcp("files", strings.Repeat("a", 70)+"b")}} {
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
	for _, raw := range []string{"mcp:files:read:extra", "builtIn:%73hell", "shell", "a2a:with space"} {
		if _, err := ParseRef(raw); err == nil {
			t.Errorf("accepted noncanonical reference %q", raw)
		}
	}
}
