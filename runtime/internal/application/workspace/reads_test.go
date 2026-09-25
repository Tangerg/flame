package workspace

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestNewDiscoveryRequiresCompleteDependencies(t *testing.T) {
	scope := newScope(t, "", "", testPaths{})
	for _, test := range []struct {
		name      string
		scope     *Scope
		catalog   Catalog
		documents AgentDocFinder
	}{
		{name: "scope", catalog: workspaceCatalogStub{}, documents: staticAgentDocFinder{}},
		{name: "catalog", scope: scope, documents: staticAgentDocFinder{}},
		{name: "typed nil catalog", scope: scope, catalog: (*workspaceCatalogStub)(nil), documents: staticAgentDocFinder{}},
		{name: "documents", scope: scope, catalog: workspaceCatalogStub{}},
		{name: "typed nil documents", scope: scope, catalog: workspaceCatalogStub{}, documents: (*staticAgentDocFinder)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if discovery, err := NewDiscovery(test.scope, test.catalog, test.documents); err == nil || discovery != nil {
				t.Fatalf("NewDiscovery = (%v, %v), want incomplete construction rejected", discovery, err)
			}
		})
	}
}

func newDiscovery(t *testing.T, scope *Scope, catalog Catalog, documents AgentDocFinder) *Discovery {
	t.Helper()
	discovery, err := NewDiscovery(scope, catalog, documents)
	if err != nil {
		t.Fatal(err)
	}
	return discovery
}

type staticAgentDocFinder struct{ files []AgentDocFile }

func (s staticAgentDocFinder) Find(context.Context, string, string) ([]AgentDocFile, error) {
	return s.files, nil
}

type workspaceCatalogStub struct {
	sessions []session.Session
	resolved map[string]Resolved
}

func (w workspaceCatalogStub) List(context.Context) ([]session.Session, error) {
	return w.sessions, nil
}

func (w workspaceCatalogStub) InspectWorkspace(path string) (Resolved, error) {
	resolved, ok := w.resolved[path]
	if !ok {
		return Resolved{}, errors.New("workspace is not configured")
	}
	return resolved, nil
}

func TestWorkspacesFromSessions(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	workspaces := workspacesFromSessions([]session.Session{
		testsupport.MustRestoreSession(session.Snapshot{ID: "s1", Workspace: testsupport.MustWorkspace("/a/proj"), UpdatedAt: t0}),
		testsupport.MustRestoreSession(session.Snapshot{ID: "s2", Workspace: testsupport.MustWorkspace("/a/proj"), UpdatedAt: t0.Add(2 * time.Hour)}),
		testsupport.MustRestoreSession(session.Snapshot{ID: "s3", Workspace: testsupport.MustWorkspace("/b/other"), UpdatedAt: t0.Add(time.Hour)}),
		testsupport.MustRestoreSession(session.Snapshot{ID: "s4", Workspace: testsupport.MustWorkspace("/c/zeta"), UpdatedAt: t0}),
		testsupport.MustRestoreSession(session.Snapshot{ID: "s5", Workspace: testsupport.MustWorkspace("/c/alpha"), UpdatedAt: t0}),
	})
	if len(workspaces) != 4 {
		t.Fatalf("workspaces = %d, want 4", len(workspaces))
	}
	if workspaces[0].Path != "/a/proj" || workspaces[0].Name != "proj" || workspaces[0].SessionCount != 2 {
		t.Fatalf("first workspace = %+v", workspaces[0])
	}
	if !workspaces[0].LastActiveAt.Equal(t0.Add(2 * time.Hour)) {
		t.Fatalf("last active = %v", workspaces[0].LastActiveAt)
	}
	if workspaces[1].Path != "/b/other" || workspaces[1].SessionCount != 1 {
		t.Fatalf("second workspace = %+v", workspaces[1])
	}
	if workspaces[2].Path != "/c/alpha" || workspaces[3].Path != "/c/zeta" {
		t.Fatalf("equal-activity workspaces = %+v, want canonical path order", workspaces[2:])
	}
}

func TestWorkspacesCollapseLiveAliasesIntoCanonicalIdentity(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	catalog := workspaceCatalogStub{
		sessions: []session.Session{
			testsupport.MustRestoreSession(session.Snapshot{ID: "s1", Workspace: testsupport.MustWorkspace("/aliases/one"), UpdatedAt: t0}),
			testsupport.MustRestoreSession(session.Snapshot{ID: "s2", Workspace: testsupport.MustWorkspace("/aliases/two"), UpdatedAt: t0.Add(time.Hour)}),
		},
		resolved: map[string]Resolved{
			"/aliases/one": {Path: "/real/project", ProjectRoot: "/real", Missing: false},
			"/aliases/two": {Path: "/real/project", ProjectRoot: "/real", Missing: false},
		},
	}
	discovery := newDiscovery(t, newScope(t, "", "", testPaths{}), catalog, staticAgentDocFinder{})

	workspaces, err := discovery.Workspaces(t.Context())
	if err != nil {
		t.Fatalf("Workspaces: %v", err)
	}
	if len(workspaces) != 1 {
		t.Fatalf("workspaces = %+v, want one canonical identity", workspaces)
	}
	workspace := workspaces[0]
	if workspace.Path != "/real/project" || workspace.ProjectRoot != "/real" || workspace.Name != "project" ||
		workspace.SessionCount != 2 || !workspace.LastActiveAt.Equal(t0.Add(time.Hour)) {
		t.Fatalf("workspace = %+v, want merged canonical summary", workspace)
	}
}

func TestResolvedWorkspaceAndSummaryValidateExactIdentity(t *testing.T) {
	active := time.Unix(1, 0).UTC()
	valid := Summary{
		Name: "work", Path: "/repo/work", ProjectRoot: "/repo",
		SessionCount: 1, LastActiveAt: active,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid Summary.Validate() error = %v", err)
	}
	for name, summary := range map[string]Summary{
		"relative path":  {Name: "work", Path: "repo/work", ProjectRoot: "/repo", SessionCount: 1, LastActiveAt: active},
		"outside root":   {Name: "work", Path: "/other/work", ProjectRoot: "/repo", SessionCount: 1, LastActiveAt: active},
		"noncanonical":   {Name: "work", Path: "/repo/../repo/work", ProjectRoot: "/repo", SessionCount: 1, LastActiveAt: active},
		"wrong name":     {Name: "other", Path: "/repo/work", ProjectRoot: "/repo", SessionCount: 1, LastActiveAt: active},
		"empty catalog":  {Name: "work", Path: "/repo/work", ProjectRoot: "/repo", LastActiveAt: active},
		"missing active": {Name: "work", Path: "/repo/work", ProjectRoot: "/repo", SessionCount: 1},
	} {
		t.Run(name, func(t *testing.T) {
			if err := summary.Validate(); err == nil {
				t.Fatal("Summary.Validate() error = nil, want invalid identity")
			}
		})
	}
}

func TestWorkspaceDiscoveryRejectsInvalidOrContradictoryInspection(t *testing.T) {
	t0 := time.Unix(1, 0).UTC()
	sessions := []session.Session{
		testsupport.MustRestoreSession(session.Snapshot{ID: "s1", Workspace: testsupport.MustWorkspace("/aliases/one"), UpdatedAt: t0}),
		testsupport.MustRestoreSession(session.Snapshot{ID: "s2", Workspace: testsupport.MustWorkspace("/aliases/two"), UpdatedAt: t0}),
	}
	for name, resolved := range map[string]map[string]Resolved{
		"outside root": {
			"/aliases/one": {Path: "/outside", ProjectRoot: "/repo"},
		},
		"contradictory aliases": {
			"/aliases/one": {Path: "/real/project", ProjectRoot: "/real"},
			"/aliases/two": {Path: "/real/project", ProjectRoot: "/real/project"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			catalog := workspaceCatalogStub{sessions: sessions, resolved: resolved}
			discovery := newDiscovery(t, newScope(t, "", "", testPaths{}), catalog, staticAgentDocFinder{})
			if _, err := discovery.Workspaces(t.Context()); err == nil {
				t.Fatal("Workspaces accepted invalid inspection")
			}
		})
	}

	catalog := workspaceCatalogStub{resolved: map[string]Resolved{
		"/requested": {Path: "/other", ProjectRoot: "/repo"},
	}}
	if _, err := newDiscovery(t, newScope(t, "", "", testPaths{}), catalog, staticAgentDocFinder{}).Resolve("/requested"); err == nil {
		t.Fatal("Resolve accepted invalid inspection")
	}
}

func TestAgentDocsPreservesDiscoveryProvenance(t *testing.T) {
	finder := staticAgentDocFinder{files: []AgentDocFile{
		{Path: "/home/.flame/AGENTS.md", Content: "home", Scope: AgentDocScopeHome},
		{Path: "/repo/AGENTS.md", Content: "root", Scope: AgentDocScopeProjectRoot},
		{Path: "/repo/pkg/AGENTS.md", Content: "leaf", Scope: AgentDocScopeCWD},
	}}
	discovery := newDiscovery(t, newScope(t, "", "/home", testPaths{}), workspaceCatalogStub{}, finder)

	docs, err := discovery.AgentDocs(t.Context(), "/repo/pkg")
	if err != nil {
		t.Fatalf("AgentDocs: %v", err)
	}
	if len(docs) != 3 || docs[0].Scope != AgentDocScopeHome || docs[1].Scope != AgentDocScopeProjectRoot || docs[2].Scope != AgentDocScopeCWD {
		t.Fatalf("AgentDocs = %+v, want finder scopes unchanged", docs)
	}
}

func TestAgentDocsRejectsUnknownDiscoveryProvenance(t *testing.T) {
	finder := staticAgentDocFinder{files: []AgentDocFile{{Path: "/repo/AGENTS.md", Content: "rule", Scope: "other"}}}
	discovery := newDiscovery(t, newScope(t, "", "/home", testPaths{}), workspaceCatalogStub{}, finder)

	if _, err := discovery.AgentDocs(t.Context(), "/repo"); err == nil {
		t.Fatal("AgentDocs accepted an unknown scope")
	}
}

// TestFilePagesUseATotalOrderAndBindTheCompleteQuery covers the workspace.files
// query properties: directories precede files and paths make the order total; a
// next page seeks strictly past the previous sort key even if that row was
// deleted; and every normalized filter belongs to the cursor identity, so a
// cursor cannot silently continue a different workspace listing.
func TestFilePagesUseATotalOrderAndBindTheCompleteQuery(t *testing.T) {
	filters := []string{"/repo", "", "", "true", "false"}
	entries := []FileEntry{
		{Path: "c.txt", Kind: FileEntryFile},
		{Path: "docs", Kind: FileEntryDir},
		{Path: "a.txt", Kind: FileEntryFile},
		{Path: "b.txt", Kind: FileEntryFile},
	}

	first, cursor, err := pageFileEntries(entries, filters, "", explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first) != 2 || first[0].Path != "docs" || first[1].Path != "a.txt" || cursor == "" {
		t.Fatalf("first page = %+v, cursor %q; want docs, a.txt and a cursor", first, cursor)
	}

	// a.txt was the anchor and disappears between reads. Continuation uses its
	// sort-key value, not row existence, so b.txt and c.txt remain reachable.
	second, next, err := pageFileEntries([]FileEntry{
		{Path: "c.txt", Kind: FileEntryFile},
		{Path: "docs", Kind: FileEntryDir},
		{Path: "b.txt", Kind: FileEntryFile},
	}, filters, cursor, explicitPageLimit(t, 2))

	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second) != 2 || second[0].Path != "b.txt" || second[1].Path != "c.txt" || next != "" {
		t.Fatalf("second page = %+v, cursor %q; want b.txt, c.txt and no cursor", second, next)
	}

	otherQuery := []string{"/repo", "docs", "", "true", "false"}
	if _, _, err := pageFileEntries(entries, otherQuery, cursor, explicitPageLimit(t, 2)); !errors.Is(err, ErrPageCursor) {
		t.Fatalf("cross-query cursor err = %v, want ErrPageCursor", err)
	}
}
