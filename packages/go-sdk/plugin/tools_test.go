package plugin

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
)

func TestInvocationIdentityComesOnlyFromHostMetadata(t *testing.T) {
	for _, id := range []string{"", "\xff", strings.Repeat("a", 257)} {
		if _, err := WithInvocation(t.Context(), extension.Invocation{ID: id, Origin: extension.Model, Target: extension.Target{Scope: "installation", Reference: "owner"}}); err == nil {
			t.Fatalf("invalid logical identity was admitted: %v", id)
		}
	}
	ctx, err := WithInvocation(t.Context(), extension.Invocation{ID: "logical-call", Origin: extension.Model, Target: extension.Target{Scope: "installation", Reference: "owner"}})
	if err != nil {
		t.Fatal(err)
	}
	if id, err := InvocationID(ctx); err != nil || id != "logical-call" {
		t.Fatalf("logical identity = %q, %v", id, err)
	}
	if id, err := ToolCommandID(ctx); err != nil || id != "tool:logical-call" {
		t.Fatalf("Tool receipt identity = %q, %v", id, err)
	}
	ctx, err = WithInvocation(t.Context(), extension.Invocation{ID: strings.Repeat("a", 252), Origin: extension.Model, Target: extension.Target{Scope: "installation", Reference: "owner"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ToolCommandID(ctx); err == nil {
		t.Fatal("Tool receipt identity exceeded the command bound")
	}
}
