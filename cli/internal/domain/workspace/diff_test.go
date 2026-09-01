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

	if rows, explicit, err := DefaultDiffRowLimit().Rows(); err != nil || explicit || rows != 0 {
		t.Fatalf("default Rows = (%d, %t, %v), want absent", rows, explicit, err)
	}
	for _, rows := range []int{0, -1} {
		if _, err := NewDiffRowLimit(rows); err == nil {
			t.Fatalf("NewDiffRowLimit(%d) returned no error", rows)
		}
	}
	if err := (DiffRowLimit{}).Validate(); err == nil {
		t.Fatal("zero DiffRowLimit was accepted")
	}
	if err := (DiffRowLimit{kind: defaultRequestLimit, rows: 1}).Validate(); err == nil {
		t.Fatal("default DiffRowLimit carrying rows was accepted")
	}
}
