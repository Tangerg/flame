package session

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

func TestCatalogFilterOwnsNormalizedSearchWorkspaceAndCursorIdentity(t *testing.T) {
	t.Parallel()
	all := AllCatalogEntries()
	if err := all.Validate(); err != nil || all.CursorIdentity() != nil {
		t.Fatalf("all filter = (%+v, %v)", all, err)
	}
	filtered, err := NewCatalogFilter("  ReLeAsE %_  ", mustCatalogWorkspace(t, "/repo"))
	if err != nil {
		t.Fatal(err)
	}
	if search, present := filtered.Search(); !present || search != "release %_" {
		t.Fatalf("search = (%q, %t)", search, present)
	}
	if workspace, present := filtered.WorkspacePath(); !present || workspace != "/repo" {
		t.Fatalf("workspace = (%q, %t)", workspace, present)
	}
	wantIdentity := []string{"search", "release %_", "workspace", "/repo"}
	if got := filtered.CursorIdentity(); len(got) != len(wantIdentity) {
		t.Fatalf("cursor identity = %v, want %v", got, wantIdentity)
	} else {
		for index := range wantIdentity {
			if got[index] != wantIdentity[index] {
				t.Fatalf("cursor identity = %v, want %v", got, wantIdentity)
			}
		}
	}
}

func TestCatalogFilterRejectsAbsentOversizedAndCorruptPredicates(t *testing.T) {
	t.Parallel()
	invalidUTF8 := string([]byte{0xff})
	tests := []struct {
		name      string
		search    string
		workspace *Workspace
	}{
		{name: "absent"},
		{name: "whitespace only", search: "  \t "},
		{name: "oversized", search: strings.Repeat("界", MaximumCatalogSearchCharacters+1)},
		{name: "invalid utf8", search: invalidUTF8},
		{name: "nul", search: "bad\x00query"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if value, err := NewCatalogFilter(test.search, test.workspace); err == nil || value != (CatalogFilter{}) {
				t.Fatalf("NewCatalogFilter = (%+v, %v), want zero/error", value, err)
			}
		})
	}
	if utf8.RuneCountInString(strings.Repeat("界", MaximumCatalogSearchCharacters)) != MaximumCatalogSearchCharacters {
		t.Fatal("test fixture does not exercise character-count boundary")
	}
	if _, err := NewCatalogFilter(strings.Repeat("界", MaximumCatalogSearchCharacters), nil); err != nil {
		t.Fatalf("exact maximum search: %v", err)
	}
	// A filter has two sources, and both settle which predicates its mode
	// carries; only the unconstructed one can reach a read boundary.
	if err := (CatalogFilter{}).Validate(); err == nil {
		t.Fatal("zero filter accepted at a read boundary")
	}
	for _, path := range []string{"relative", "/repo/../repo"} {
		if _, err := NewWorkspace(path); err == nil {
			t.Fatalf("NewWorkspace(%q) accepted a non-canonical path", path)
		}
	}
}

func mustCatalogWorkspace(t *testing.T, path string) *Workspace {
	t.Helper()
	value, err := NewWorkspace(path)
	if err != nil {
		t.Fatal(err)
	}
	return &value
}

func TestCatalogAnchorAndReadRejectPrimitiveSentinelStates(t *testing.T) {
	t.Parallel()
	updatedAt := time.Unix(10, 20).UTC()
	anchor, err := NewCatalogAnchor(true, updatedAt, "ses_1")
	if err != nil {
		t.Fatal(err)
	}
	read, err := NewCatalogRead(AllCatalogEntries(), &anchor, 3)
	if err != nil {
		t.Fatal(err)
	}
	got, present := read.After()
	if !present || got != anchor || read.Limit() != 3 {
		t.Fatalf("read = {after:%+v present:%t limit:%d}", got, present, read.Limit())
	}
	for _, id := range []string{
		"", " ses_1", "ses_ one", "ses_\u200bhidden",
		strings.Repeat("界", runtimeidentity.MaximumResourceCharacters+1),
	} {
		if _, err := NewCatalogAnchor(true, updatedAt, id); err == nil {
			t.Fatalf("NewCatalogAnchor accepted identity %q", id)
		}
	}
	if _, err := NewCatalogAnchor(true, time.Time{}, "ses_1"); err == nil {
		t.Fatal("NewCatalogAnchor accepted a zero update time")
	}
	if err := (CatalogAnchor{}).Validate(); err == nil {
		t.Fatal("zero anchor accepted at a read boundary")
	}
	for _, limit := range []int{-1, 0} {
		if _, err := NewCatalogRead(AllCatalogEntries(), nil, limit); err == nil {
			t.Fatalf("catalog read accepted limit %d", limit)
		}
	}
	if _, err := NewCatalogRead(CatalogFilter{}, nil, 1); err == nil {
		t.Fatal("catalog read accepted a zero filter")
	}
	if err := (CatalogRead{}).Validate(); err == nil {
		t.Fatalf("zero catalog read error = %v", err)
	}
}
