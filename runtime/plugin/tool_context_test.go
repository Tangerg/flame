package plugin

import (
	"testing"

	scopemcp "github.com/Tangerg/scope/mcp"
)

func TestToolContextPreservesTheCapturedOwnerMetadata(t *testing.T) {
	original := map[string]any{"calling-owner": "original"}
	ctx := scopemcp.WithRequestMeta(t.Context(), original)
	want := ToolContext{CWD: "/captured/workspace", EmbeddingSpace: "captured-space"}
	prepared, err := WithToolContext(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := ToolContextFromContext(prepared)
	if err != nil || actual != want {
		t.Fatalf("captured context = %+v, %v", actual, err)
	}
	if scopemcp.RequestMetaFromContext(prepared)["calling-owner"] != "original" {
		t.Fatal("Tool context replaced the calling owner metadata")
	}
	if _, mutated := original[ToolContextMetadataKey]; mutated {
		t.Fatal("Tool context mutated another caller's metadata")
	}
	for _, value := range []any{nil, map[string]any{"cwd": ""}, map[string]any{"cwd": "/captured", "currentSession": "forged"}} {
		if _, err := ToolContextFromContext(scopemcp.WithRequestMeta(t.Context(), map[string]any{ToolContextMetadataKey: value})); err == nil {
			t.Fatalf("invalid context accepted: %+v", value)
		}
	}
}
