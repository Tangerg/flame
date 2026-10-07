package workspace

import (
	"errors"
	"testing"
)

func TestWorkspaceFileReadPoliciesOwnDefaultAndClamp(t *testing.T) {
	t.Run("head lines", func(t *testing.T) {
		if got := DefaultHeadLineLimit().Lines(); got != defaultFileHeadLines {
			t.Fatalf("default Lines = %d, want %d", got, defaultFileHeadLines)
		}
		large, err := NewHeadLineLimit(maxFileHeadLines + 1)
		if err != nil {
			t.Fatal(err)
		}
		if got := large.Lines(); got != maxFileHeadLines {
			t.Fatalf("clamped Lines = %d, want %d", got, maxFileHeadLines)
		}
		for _, lines := range []int{0, -1} {
			if _, err := NewHeadLineLimit(lines); !errors.Is(err, ErrInvalidFileRange) {
				t.Fatalf("NewHeadLineLimit(%d) = %v", lines, err)
			}
		}
	})

	t.Run("grep matches", func(t *testing.T) {
		if got := DefaultGrepResultLimit().Matches(); got != DefaultGrepLimit {
			t.Fatalf("default Matches = %d, want %d", got, DefaultGrepLimit)
		}
		large, err := NewGrepResultLimit(MaxGrepLimit + 1)
		if err != nil {
			t.Fatal(err)
		}
		if got := large.Matches(); got != MaxGrepLimit {
			t.Fatalf("clamped Matches = %d, want %d", got, MaxGrepLimit)
		}
		for _, matches := range []int{0, -1} {
			if _, err := NewGrepResultLimit(matches); !errors.Is(err, ErrInvalidGrepLimit) {
				t.Fatalf("NewGrepResultLimit(%d) = %v", matches, err)
			}
		}
	})

	t.Run("read bytes", func(t *testing.T) {
		if got := DefaultFileReadByteLimit().Bytes(); got != DefaultFileReadBytes {
			t.Fatalf("default Bytes = %d, want %d", got, DefaultFileReadBytes)
		}
		large, err := NewFileReadByteLimit(MaxFileReadBytes + 1)
		if err != nil {
			t.Fatal(err)
		}
		if got := large.Bytes(); got != MaxFileReadBytes {
			t.Fatalf("clamped Bytes = %d, want %d", got, MaxFileReadBytes)
		}
		for _, bytes := range []int{0, -1} {
			if _, err := NewFileReadByteLimit(bytes); !errors.Is(err, ErrInvalidFileReadLimit) {
				t.Fatalf("NewFileReadByteLimit(%d) = %v", bytes, err)
			}
		}
	})
}

func TestFileLineRangeIsClosedOverWholeTailAndBoundedWindows(t *testing.T) {
	if start, end := WholeFileRange().Bounds(); start != 0 || end != 0 {
		t.Fatalf("whole Bounds = (%d, %d)", start, end)
	}
	tail, err := NewFileTailRange(3)
	if err != nil {
		t.Fatal(err)
	}
	if start, end := tail.Bounds(); start != 3 || end != 0 {
		t.Fatalf("tail Bounds = (%d, %d)", start, end)
	}
	bounded, err := NewFileLineRange(3, 5)
	if err != nil {
		t.Fatal(err)
	}
	if start, end := bounded.Bounds(); start != 3 || end != 5 {
		t.Fatalf("bounded Bounds = (%d, %d)", start, end)
	}
	for _, test := range []struct{ start, end int }{{0, 1}, {2, 1}} {
		if _, err := NewFileLineRange(test.start, test.end); !errors.Is(err, ErrInvalidFileRange) {
			t.Fatalf("NewFileLineRange(%d, %d) = %v", test.start, test.end, err)
		}
	}
	if _, err := NewFileTailRange(0); !errors.Is(err, ErrInvalidFileRange) {
		t.Fatalf("NewFileTailRange(0) = %v", err)
	}
}
