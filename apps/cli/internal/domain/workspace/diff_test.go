package workspace

import (
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
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

func TestDiffRowLimitSeparatesDefaultFromExplicitRows(t *testing.T) {
	t.Parallel()

	if rows := DefaultDiffRowLimit().Rows(); rows != 0 {
		t.Fatalf("default Rows = %d, want absent", rows)
	}
	for _, rows := range []int{0, -1} {
		if _, err := NewDiffRowLimit(rows); err == nil {
			t.Fatalf("NewDiffRowLimit(%d) returned no error", rows)
		}
	}
	limit, err := NewDiffRowLimit(42)
	if err != nil || limit.Rows() != 42 {
		t.Fatalf("explicit row limit = %d, %v", limit.Rows(), err)
	}
}
