package fingerprint

import "testing"

func TestStringsPreservesThePersistedFraming(t *testing.T) {
	if got := Strings("a", "bc").String(); got != "5310a58788781ab25d5ad7c3f85035824b4eb7bdfa394e0ac2186271472b5492" {
		t.Fatalf("fingerprint = %s", got)
	}
	for _, pair := range [][2][]string{
		{{"a", "bc"}, {"ab", "c"}},
		{{}, {""}},
		{{"a"}, {"a", ""}},
		{{"1:", "x"}, {"1", ":x"}},
		{{"é", "x"}, {"x", "é"}},
	} {
		if Strings(pair[0]...) == Strings(pair[1]...) {
			t.Fatalf("field boundaries or order collapsed: %q and %q", pair[0], pair[1])
		}
	}
}
