package plugin

import (
	"strings"
	"testing"

	scopemcp "github.com/Tangerg/scope/mcp"
)

func TestInvocationIdentityComesOnlyFromHostMetadata(t *testing.T) {
	for _, id := range []any{nil, 42, "", "\xff", strings.Repeat("a", 257)} {
		ctx := scopemcp.WithRequestMeta(t.Context(), map[string]any{InvocationKey: id})
		if _, err := InvocationID(ctx); err == nil {
			t.Fatalf("invalid logical identity was admitted: %v", id)
		}
	}
	ctx := scopemcp.WithRequestMeta(t.Context(), map[string]any{InvocationKey: "logical-call"})
	if id, err := InvocationID(ctx); err != nil || id != "logical-call" {
		t.Fatalf("logical identity = %q, %v", id, err)
	}
	if id, err := ToolCommandID(ctx); err != nil || id != "tool:logical-call" {
		t.Fatalf("Tool receipt identity = %q, %v", id, err)
	}
	ctx = scopemcp.WithRequestMeta(t.Context(), map[string]any{InvocationKey: strings.Repeat("a", 252)})
	if _, err := ToolCommandID(ctx); err == nil {
		t.Fatal("Tool receipt identity exceeded the command bound")
	}
}
