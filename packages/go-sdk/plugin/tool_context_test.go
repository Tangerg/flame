package plugin

import (
	"testing"

	"context"

	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestToolContextPreservesTheCapturedOwnerMetadata(t *testing.T) {
	original := sdk.Meta{}
	if err := original.Set("calling-owner", "original"); err != nil {
		t.Fatal(err)
	}
	type ownerKey struct{}
	ctx := context.WithValue(t.Context(), ownerKey{}, "original")
	want := ToolContext{CWD: "/captured/workspace", EmbeddingSpace: "captured-space"}
	prepared, err := WithToolContext(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := ToolContextFromContext(prepared)
	if err != nil || actual != want {
		t.Fatalf("captured context = %+v, %v", actual, err)
	}
	if prepared.Value(ownerKey{}) != "original" {
		t.Fatal("Tool context replaced the calling owner metadata")
	}
	if _, mutated := original[ToolContextMetadataKey]; mutated {
		t.Fatal("Tool context mutated another caller's metadata")
	}
	var metadata sdk.Meta
	if err := metadata.Set(ToolContextMetadataKey, want); err != nil {
		t.Fatal(err)
	}
	if decoded, err := ToolContextFromMetadata(metadata); err != nil || decoded != want {
		t.Fatalf("metadata roundtrip = %+v, %v", decoded, err)
	}
	if _, err := ToolContextFromContext(t.Context()); err == nil {
		t.Fatal("missing Tool context accepted")
	}
	for _, value := range []any{nil, map[string]any{"cwd": ""}, map[string]any{"cwd": "/captured", "currentSession": "forged"}} {
		var metadata sdk.Meta
		if err := metadata.Set(ToolContextMetadataKey, value); err != nil {
			t.Fatal(err)
		}
		if _, err := ToolContextFromMetadata(metadata); err == nil {
			t.Fatalf("invalid context accepted: %+v", value)
		}
	}
}
