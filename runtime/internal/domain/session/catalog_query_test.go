package session

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/resourceidentity"
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
		{name: "relative workspace", workspace: &Workspace{path: "relative"}},
		{name: "unclean workspace", workspace: &Workspace{path: "/repo/../repo"}},
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
	for _, corrupt := range []CatalogFilter{
		{},
		{kind: allCatalogEntries, search: "leak"},
		{kind: searchCatalogEntries},
		{kind: workspaceCatalogEntries, workspace: "/repo", search: "leak"},
	} {
		if err := corrupt.Validate(); err == nil {
			t.Fatalf("corrupt filter accepted: %+v", corrupt)
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
	for _, badAnchor := range []CatalogAnchor{
		{},
		{favorite: true, updatedAt: updatedAt},
		{updatedAt: updatedAt, id: " ses_1"},
		{updatedAt: updatedAt, id: "ses_ one"},
		{updatedAt: updatedAt, id: "ses_\u200bhidden"},
		{updatedAt: updatedAt, id: strings.Repeat("界", resourceidentity.MaximumCharacters+1)},
	} {
		if err := badAnchor.Validate(); err == nil {
			t.Fatalf("corrupt anchor accepted: %+v", badAnchor)
		}
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
