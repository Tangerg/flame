package workspace

import (
	"errors"
	"testing"
)

func TestWorkspaceFileReadPoliciesOwnDefaultClampAndInvalidState(t *testing.T) {
	t.Run("head lines", func(t *testing.T) {
		if got, err := DefaultHeadLineLimit().Lines(); err != nil || got != defaultFileHeadLines {
			t.Fatalf("default Lines = (%d, %v), want %d", got, err, defaultFileHeadLines)
		}
		large, err := NewHeadLineLimit(maxFileHeadLines + 1)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := large.Lines(); err != nil || got != maxFileHeadLines {
			t.Fatalf("clamped Lines = (%d, %v), want %d", got, err, maxFileHeadLines)
		}
		for _, lines := range []int{0, -1} {
			if _, err := NewHeadLineLimit(lines); !errors.Is(err, ErrInvalidFileRange) {
				t.Fatalf("NewHeadLineLimit(%d) = %v", lines, err)
			}
		}
		if _, err := (HeadLineLimit{lines: 1}).Lines(); !errors.Is(err, ErrInvalidFileRange) {
			t.Fatalf("corrupt head limit = %v", err)
		}
	})

	t.Run("grep matches", func(t *testing.T) {
		if got, err := DefaultGrepResultLimit().Matches(); err != nil || got != DefaultGrepLimit {
			t.Fatalf("default Matches = (%d, %v), want %d", got, err, DefaultGrepLimit)
		}
		large, err := NewGrepResultLimit(MaxGrepLimit + 1)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := large.Matches(); err != nil || got != MaxGrepLimit {
			t.Fatalf("clamped Matches = (%d, %v), want %d", got, err, MaxGrepLimit)
		}
		for _, matches := range []int{0, -1} {
			if _, err := NewGrepResultLimit(matches); !errors.Is(err, ErrInvalidGrepLimit) {
				t.Fatalf("NewGrepResultLimit(%d) = %v", matches, err)
			}
		}
		if _, err := (GrepResultLimit{matches: 1}).Matches(); !errors.Is(err, ErrInvalidGrepLimit) {
			t.Fatalf("corrupt grep limit = %v", err)
		}
	})

	t.Run("read bytes", func(t *testing.T) {
		if got, err := DefaultFileReadByteLimit().Bytes(); err != nil || got != DefaultFileReadBytes {
			t.Fatalf("default Bytes = (%d, %v), want %d", got, err, DefaultFileReadBytes)
		}
		large, err := NewFileReadByteLimit(MaxFileReadBytes + 1)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := large.Bytes(); err != nil || got != MaxFileReadBytes {
			t.Fatalf("clamped Bytes = (%d, %v), want %d", got, err, MaxFileReadBytes)
		}
		for _, bytes := range []int{0, -1} {
			if _, err := NewFileReadByteLimit(bytes); !errors.Is(err, ErrInvalidFileReadLimit) {
				t.Fatalf("NewFileReadByteLimit(%d) = %v", bytes, err)
			}
		}
		if _, err := (FileReadByteLimit{bytes: 1}).Bytes(); !errors.Is(err, ErrInvalidFileReadLimit) {
			t.Fatalf("corrupt byte limit = %v", err)
		}
	})
}

func TestFileLineRangeIsClosedOverWholeTailAndBoundedWindows(t *testing.T) {
	whole := WholeFileRange()
	if start, end, err := whole.Bounds(); err != nil || start != 0 || end != 0 {
		t.Fatalf("whole Bounds = (%d, %d, %v)", start, end, err)
	}
	tail, err := NewFileTailRange(3)
	if err != nil {
		t.Fatal(err)
	}
	if start, end, err := tail.Bounds(); err != nil || start != 3 || end != 0 {
		t.Fatalf("tail Bounds = (%d, %d, %v)", start, end, err)
	}
	bounded, err := NewFileLineRange(3, 5)
	if err != nil {
		t.Fatal(err)
	}
	if start, end, err := bounded.Bounds(); err != nil || start != 3 || end != 5 {
		t.Fatalf("bounded Bounds = (%d, %d, %v)", start, end, err)
	}
	for _, test := range []struct{ start, end int }{{0, 1}, {2, 1}} {
		if _, err := NewFileLineRange(test.start, test.end); !errors.Is(err, ErrInvalidFileRange) {
			t.Fatalf("NewFileLineRange(%d, %d) = %v", test.start, test.end, err)
		}
	}
	if _, _, err := (FileLineRange{kind: fileLineRangeTail, start: 1, end: 2}).Bounds(); !errors.Is(err, ErrInvalidFileRange) {
		t.Fatalf("corrupt tail range = %v", err)
	}
}
