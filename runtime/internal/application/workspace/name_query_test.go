package workspace

import (
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	"slices"
	"testing"
)

func TestFileNameSuggestionsRankAllMatchesBeforePaging(t *testing.T) {
	entries := []FileEntry{{Path: "ab", Kind: FileEntryFile}, {Path: "src/ab", Kind: FileEntryFile}, {Path: "a/b", Kind: FileEntryFile}, {Path: "abdir", Kind: FileEntryDir}, {Path: "unrelated", Kind: FileEntryFile}}
	ranked, err := rankedFileNames(t.Context(), entries, "ab")
	if err != nil {
		t.Fatal(err)
	}
	page, _, err := pageFileEntries(ranked, []string{"ab"}, "", pagination.DefaultLimit())
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, e := range page {
		paths = append(paths, e.Path)
	}
	if !slices.Equal(paths, []string{"ab", "src/ab", "a/b"}) {
		t.Fatalf("ranked paths %v", paths)
	}
	none, err := rankedFileNames(t.Context(), entries, "z")
	if err != nil || len(none) != 0 {
		t.Fatalf("non-matching query: %v %v", none, err)
	}
	limited, _, err := pageFileEntries(ranked, []string{"ab"}, "", explicitPageLimit(t, 1))
	if err != nil || len(limited) != 1 || limited[0].Path != "ab" {
		t.Fatalf("capped page %v, %v", limited, err)
	}

}
