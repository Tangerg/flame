package workspace

import "testing"

func TestFileRequestPoliciesKeepDefaultsAndExplicitValuesDistinct(t *testing.T) {
	t.Parallel()

	if lines, err := DefaultHeadLineLimit().Lines(); err != nil || lines != DefaultHeadLines {
		t.Fatalf("default head lines = (%d, %v)", lines, err)
	}
	if matches, err := DefaultSearchResultLimit().Matches(); err != nil || matches != DefaultSearchMatches {
		t.Fatalf("default search matches = (%d, %v)", matches, err)
	}
	if bytes, err := DefaultReadByteLimit().Bytes(); err != nil || bytes != DefaultReadBytes {
		t.Fatalf("default read bytes = (%d, %v)", bytes, err)
	}
	for _, test := range []struct {
		name string
		new  func(int) error
	}{
		{name: "head", new: func(value int) error { _, err := NewHeadLineLimit(value); return err }},
		{name: "search", new: func(value int) error { _, err := NewSearchResultLimit(value); return err }},
		{name: "bytes", new: func(value int) error { _, err := NewReadByteLimit(value); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, value := range []int{0, -1} {
				if err := test.new(value); err == nil {
					t.Fatalf("constructor accepted %d", value)
				}
			}
		})
	}
	if _, err := (HeadLineLimit{}).Lines(); err == nil {
		t.Fatal("zero head limit was accepted")
	}
	if _, err := (HeadLineLimit{kind: defaultRequestLimit, lines: 1}).Lines(); err == nil {
		t.Fatal("corrupt default head limit was accepted")
	}
}

func TestReadLineRangeIsClosedOverWholeTailAndBounded(t *testing.T) {
	t.Parallel()

	if start, end, err := WholeFileReadRange().Bounds(); err != nil || start != 0 || end != 0 {
		t.Fatalf("whole range = (%d, %d, %v)", start, end, err)
	}
	tail, err := NewReadTailRange(4)
	if err != nil {
		t.Fatal(err)
	}
	if start, end, err := tail.Bounds(); err != nil || start != 4 || end != 0 {
		t.Fatalf("tail range = (%d, %d, %v)", start, end, err)
	}
	bounded, err := NewReadLineRange(4, 8)
	if err != nil {
		t.Fatal(err)
	}
	if start, end, err := bounded.Bounds(); err != nil || start != 4 || end != 8 {
		t.Fatalf("bounded range = (%d, %d, %v)", start, end, err)
	}
	if _, err := NewReadLineRange(8, 4); err == nil {
		t.Fatal("reversed range was accepted")
	}
	if _, _, err := (ReadLineRange{}).Bounds(); err == nil {
		t.Fatal("zero read range was accepted")
	}
}
