package sqlite_test

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport/sessionfixture"
)

func TestSessionCatalogStoreAppliesLiteralSearchWorkspaceAndKeysetTogether(t *testing.T) {
	store := newTempDB(t)
	fixtures := []session.Snapshot{
		{ID: "ses_percent", Title: "Alpha % milestone", Workspace: sessionfixture.MustWorkspace("/repo/one"), Favorite: true},
		{ID: "ses_plain", Title: "ALPHA release", Workspace: sessionfixture.MustWorkspace("/repo/two")},
		{ID: "ses_workspace", Title: "Other", Workspace: sessionfixture.MustWorkspace("/repo/alpha")},
		{ID: "ses_underscore", Title: "literal_name", Workspace: sessionfixture.MustWorkspace("/repo/four")},
		{ID: "ses_unmatched", Title: "Other", Workspace: sessionfixture.MustWorkspace("/repo/five")},
		{ID: "ses_unicode", Title: "ÄRGER review", Workspace: sessionfixture.MustWorkspace("/repo/six")},
	}
	for index := range fixtures {
		createdAt := time.Unix(int64(index+1), 0).UTC()
		fixtures[index].StartedAt = createdAt
		fixtures[index].UpdatedAt = createdAt
		fixtures[index].Revision = 1
		if err := store.Insert(t.Context(), sessionfixture.MustRestore(fixtures[index])); err != nil {
			t.Fatalf("insert fixture %d: %v", index, err)
		}
	}

	search, err := session.NewCatalogFilter("  alpha ", nil)
	if err != nil {
		t.Fatal(err)
	}
	searchRead, err := session.NewCatalogRead(search, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.ListPage(t.Context(), searchRead)
	if err != nil || len(first) != 2 {
		t.Fatalf("first search page = (%v, %v)", sessionIDs(first), err)
	}
	anchor, err := session.NewCatalogAnchor(
		first[len(first)-1].Favorite(),
		first[len(first)-1].UpdatedAt(),
		first[len(first)-1].ID(),
	)
	if err != nil {
		t.Fatal(err)
	}
	nextRead, err := session.NewCatalogRead(search, &anchor, 2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.ListPage(t.Context(), nextRead)
	if err != nil || len(second) != 1 {
		t.Fatalf("second search page = (%v, %v)", sessionIDs(second), err)
	}
	allMatches := append(sessionIDs(first), sessionIDs(second)...)
	assertSessionIDSet(t, allMatches, "ses_percent", "ses_plain", "ses_workspace")

	for _, test := range []struct {
		name      string
		search    string
		workspace string
		want      []string
	}{
		{name: "literal percent", search: "%", want: []string{"ses_percent"}},
		{name: "literal underscore", search: "_", want: []string{"ses_underscore"}},
		{name: "search and workspace", search: "alpha", workspace: "/repo/one", want: []string{"ses_percent"}},
		{name: "exact workspace", workspace: "/repo/two", want: []string{"ses_plain"}},
		{name: "unicode case", search: "ärger", want: []string{"ses_unicode"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var workspace *session.Workspace
			if test.workspace != "" {
				value := sessionfixture.MustWorkspace(test.workspace)
				workspace = &value
			}
			filter, err := session.NewCatalogFilter(test.search, workspace)
			if err != nil {
				t.Fatal(err)
			}
			read, err := session.NewCatalogRead(filter, nil, 10)
			if err != nil {
				t.Fatal(err)
			}
			rows, err := store.ListPage(t.Context(), read)
			if err != nil {
				t.Fatal(err)
			}
			assertSessionIDSet(t, sessionIDs(rows), test.want...)
		})
	}
}

func sessionIDs(values []session.Session) []string {
	ids := make([]string, len(values))
	for index, value := range values {
		ids[index] = value.ID()
	}
	return ids
}

func assertSessionIDSet(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("session ids = %v, want %v", got, want)
	}
	present := make(map[string]struct{}, len(got))
	for _, id := range got {
		present[id] = struct{}{}
	}
	for _, id := range want {
		if _, ok := present[id]; !ok {
			t.Fatalf("session ids = %v, missing %q", got, id)
		}
	}
}
