package extension

import "testing"

func TestResultShapeValuesCannotChangeTheVocabulary(t *testing.T) {
	values := ResultShape("").Values()
	if len(values) == 0 {
		t.Fatal("result shape vocabulary is empty")
	}
	first := ResultShape(values[0])
	values[0] = "not-a-shape"
	if !first.Valid() || ResultShape(values[0]).Valid() {
		t.Fatal("caller changed result shape admission")
	}
	for _, word := range []ResultShape{"", "SEARCH", "search ", "not-a-shape"} {
		if word.Valid() {
			t.Fatalf("unknown shape %q admitted", word)
		}
	}
}
