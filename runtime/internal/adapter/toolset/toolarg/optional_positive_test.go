package toolarg

import "testing"

func intPointer(value int) *int { return &value }

func TestPositiveIntPreservesAbsenceAndRejectsNumericSentinels(t *testing.T) {
	t.Parallel()

	if value, err := PositiveInt(nil, 8, 20, "limit"); err != nil || value != 8 {
		t.Fatalf("absent = (%d,%v), want default 8", value, err)
	}
	if value, err := PositiveInt(intPointer(12), 8, 20, "limit"); err != nil || value != 12 {
		t.Fatalf("present = (%d,%v), want 12", value, err)
	}
	for _, value := range []int{0, -1, 21} {
		if _, err := PositiveInt(intPointer(value), 8, 20, "limit"); err == nil {
			t.Fatalf("PositiveInt(%d) succeeded", value)
		}
	}
}
