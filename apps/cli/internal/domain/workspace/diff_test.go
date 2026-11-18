package workspace

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestDiffRequestRequiresStructuredFormatForRowLimit(t *testing.T) {
	t.Parallel()

	limit, err := NewDiffRowLimit(10)
	if err != nil {
		t.Fatalf("NewDiffRowLimit: %v", err)
	}
	request := DiffRequest{Workspace: "/workspace", Mode: protocol.DiffModeWorktree, Format: protocol.DiffFormatRaw, RowLimit: limit}
	if err := request.Validate(); err == nil {
		t.Fatal("raw diff accepted a structured row limit")
	}
	request.Format = protocol.DiffFormatRows
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDiffRequestRejectsTheVisualProjection(t *testing.T) {
	request := DiffRequest{Workspace: "/workspace", Format: protocol.DiffFormatVisual}
	if err := request.Validate(); err == nil {
		t.Fatal("terminal accepted a visual-only projection")
	}
}
