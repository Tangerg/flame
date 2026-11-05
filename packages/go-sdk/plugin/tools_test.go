package plugin

import (
	"strings"
	"testing"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/frame/portable"
	framemcp "github.com/Tangerg/frame/process/mcp"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
)

func TestInvocationIdentityComesOnlyFromHostMetadata(t *testing.T) {
	for _, id := range []string{"", "\xff", strings.Repeat("a", 257)} {
		if _, err := pluginmcp.WithInvocation(t.Context(), framemcp.Invocation{ID: id, Origin: extension.Model, Target: portable.Target{Scope: "installation", Reference: "owner"}}); err == nil {
			t.Fatalf("invalid logical identity was admitted: %v", id)
		}
	}
	ctx, err := pluginmcp.WithInvocation(t.Context(), framemcp.Invocation{ID: "logical-call", Origin: extension.Model, Target: portable.Target{Scope: "installation", Reference: "owner"}})
	if err != nil {
		t.Fatal(err)
	}
	if id, err := InvocationID(ctx); err != nil || id != "logical-call" {
		t.Fatalf("logical identity = %q, %v", id, err)
	}
	if id, err := ToolCommandID(ctx); err != nil || id != "tool:logical-call" {
		t.Fatalf("Tool receipt identity = %q, %v", id, err)
	}
	ctx, err = pluginmcp.WithInvocation(t.Context(), framemcp.Invocation{ID: strings.Repeat("a", 252), Origin: extension.Model, Target: portable.Target{Scope: "installation", Reference: "owner"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ToolCommandID(ctx); err == nil {
		t.Fatal("Tool receipt identity exceeded the command bound")
	}
}
