package workspace

import "testing"

func TestFileRequestPoliciesKeepDefaultsAndExplicitValuesDistinct(t *testing.T) {
	t.Parallel()

	if matches := DefaultSearchResultLimit().Matches(); matches != 0 {
		t.Fatalf("default search matches = %d, want omitted", matches)
	}
	if bytes := DefaultReadByteLimit().Bytes(); bytes != 0 {
		t.Fatalf("default read bytes = %d, want omitted", bytes)
	}
	for _, test := range []struct {
		name string
		new  func(int) error
	}{
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

}

func TestReadLineRangeIsClosedOverWholeTailAndBounded(t *testing.T) {
	t.Parallel()

	if start, end := WholeFileReadRange().Bounds(); start != 0 || end != 0 {
		t.Fatalf("whole range = (%d, %d)", start, end)
	}
	tail, err := NewReadTailRange(4)
	if err != nil {
		t.Fatal(err)
	}
	if start, end := tail.Bounds(); start != 4 || end != 0 {
		t.Fatalf("tail range = (%d, %d)", start, end)
	}
	bounded, err := NewReadLineRange(4, 8)
	if err != nil {
		t.Fatal(err)
	}
	if start, end := bounded.Bounds(); start != 4 || end != 8 {
		t.Fatalf("bounded range = (%d, %d)", start, end)
	}
	if _, err := NewReadLineRange(8, 4); err == nil {
		t.Fatal("reversed range was accepted")
	}
	if start, end := (ReadLineRange{}).Bounds(); start != 0 || end != 0 {
		t.Fatal("zero read range is not whole-file")
	}
	for _, start := range []int{0, -1} {
		if _, err := NewReadTailRange(start); err == nil {
			t.Fatalf("tail accepted start %d", start)
		}
		if _, err := NewReadLineRange(start, 5); err == nil {
			t.Fatalf("bounded range accepted start %d", start)
		}
	}
}
