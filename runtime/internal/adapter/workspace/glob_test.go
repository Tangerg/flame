package workspace

import (
	"strings"
	"testing"
)

func TestGlobBudgetAndDoubleStar(t *testing.T) {
	if _, err := compileGlob(strings.Repeat("**/", maxGlobSegments) + "x"); err == nil {
		t.Fatal("unbounded pattern admitted")
	}
	g, err := compileGlob("**/**/src/**/*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		want bool
	}{{"src/main.go", true}, {"mod/src/sub/main.go", true}, {"src/main.ts", false}, {"main.go", false}} {
		if g.matches(tc.name) != tc.want {
			t.Fatalf("wrong match for %q", tc.name)
		}
	}
}
