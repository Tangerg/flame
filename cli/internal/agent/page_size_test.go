package agent

import (
	"errors"
	"testing"
)

func TestPageSizeSeparatesDefaultFromExplicitRows(t *testing.T) {
	t.Parallel()

	if rows, err := DefaultPageSize().Rows(); err != nil || rows != DefaultPageRows {
		t.Fatalf("default Rows = (%d, %v), want %d", rows, err, DefaultPageRows)
	}
	explicit, err := NewPageSize(7)
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := explicit.Rows(); err != nil || rows != 7 {
		t.Fatalf("explicit Rows = (%d, %v), want 7", rows, err)
	}
	if explicit.kind != explicitPageSize || explicit.rows != 7 {
		t.Fatalf("explicit intent = %+v, want 7", explicit)
	}
	if defaulted := DefaultPageSize(); defaulted.kind != defaultPageSize || defaulted.rows != 0 {
		t.Fatalf("default intent = %+v, want absent", defaulted)
	}
}

func TestPageSizeRejectsNonPositiveAndOversizedRows(t *testing.T) {
	t.Parallel()
	for _, rows := range []int{-1, 0, MaximumPageRows + 1} {
		if _, err := NewPageSize(rows); !errors.Is(err, ErrInvalidPageSize) {
			t.Fatalf("NewPageSize(%d) = %v", rows, err)
		}
	}
	for _, corrupt := range []PageSize{
		{},
		{kind: defaultPageSize, rows: 1},
		{kind: explicitPageSize},
		{kind: explicitPageSize, rows: MaximumPageRows + 1},
	} {
		if _, err := corrupt.Rows(); !errors.Is(err, ErrInvalidPageSize) {
			t.Fatalf("corrupt Rows(%+v) = %v", corrupt, err)
		}
	}
}
