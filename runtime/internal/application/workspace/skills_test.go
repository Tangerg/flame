package workspace

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
)

func TestNewSkillsRequiresCompleteDiscoveryAndReview(t *testing.T) {
	scope := newScope(t, "", "", testPaths{})
	for _, test := range []struct {
		name      string
		scope     *Scope
		catalog   SkillCatalog
		curator   SkillCurator
		proposals SkillProposals
	}{
		{name: "scope", catalog: &fakeSkillCatalog{}, proposals: &fakeSkillProposals{}},
		{name: "catalog", scope: scope, proposals: &fakeSkillProposals{}},
		{name: "typed nil catalog", scope: scope, catalog: (*fakeSkillCatalog)(nil), proposals: &fakeSkillProposals{}},
		{name: "proposals", scope: scope, catalog: &fakeSkillCatalog{}},
		{name: "typed nil proposals", scope: scope, catalog: &fakeSkillCatalog{}, proposals: (*fakeSkillProposals)(nil)},
		{name: "typed nil curator", scope: scope, catalog: &fakeSkillCatalog{}, proposals: &fakeSkillProposals{}, curator: (*fakeSkillCurator)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if useCases, err := NewSkills(test.scope, test.catalog, test.curator, test.proposals, nil, nil); err == nil || useCases != nil {
				t.Fatalf("NewSkills = (%v, %v), want incomplete construction rejected", useCases, err)
			}
		})
	}
}

func newSkills(t *testing.T, scope *Scope, catalog SkillCatalog, curator SkillCurator, proposals SkillProposals, observations *AuthoredWatch, publish invalidation.Publish) *Skills {
	t.Helper()
	useCases, err := NewSkills(scope, catalog, curator, proposals, observations, publish)
	if err != nil {
		t.Fatal(err)
	}
	return useCases
}

func TestListUsesCatalogPort(t *testing.T) {
	catalog := &fakeSkillCatalog{
		skills: []SkillSummary{{Name: "lint", Description: "check code", Scope: SkillScopeProject}},
	}
	c := newSkills(t, newScope(t, "", "", testPaths{}), catalog, nil, &fakeSkillProposals{}, nil, nil)

	got, err := c.List(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if catalog.cwd != "/repo" {
		t.Fatalf("catalog cwd = %q", catalog.cwd)
	}
	if len(got) != 1 || got[0].Name != "lint" {
		t.Fatalf("skills = %+v", got)
	}
}

func TestListOwnsVisibleSkillOrder(t *testing.T) {
	catalog := &fakeSkillCatalog{skills: []SkillSummary{
		{Name: "zeta", Description: "Check the final result.", Scope: SkillScopeUser},
		{Name: "alpha", Description: "Inspect the project first.", Scope: SkillScopeProject},
	}}
	c := newSkills(t, newScope(t, "", "", testPaths{}), catalog, nil, &fakeSkillProposals{}, nil, nil)

	got, err := c.List(t.Context(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "zeta" {
		t.Fatalf("skills = %+v, want alpha then zeta", got)
	}
	if catalog.skills[0].Name != "zeta" {
		t.Fatal("List reordered adapter-owned storage")
	}
	got[0].Name = "caller edit"
	next, err := c.List(t.Context(), "/repo")
	if err != nil || len(next) != 2 || next[0].Name != "alpha" {
		t.Fatalf("List after caller reused result = (%+v, %v)", next, err)
	}
}

func TestListRejectsShadowedSkillLeak(t *testing.T) {
	catalog := &fakeSkillCatalog{skills: []SkillSummary{
		{Name: "review", Description: "Review the project changes.", Scope: SkillScopeProject},
		{Name: "review", Description: "Review the user changes.", Scope: SkillScopeUser},
	}}
	c := newSkills(t, newScope(t, "", "", testPaths{}), catalog, nil, &fakeSkillProposals{}, nil, nil)

	if _, err := c.List(t.Context(), "/repo"); err == nil {
		t.Fatal("List accepted two visible Skills with one precedence-resolved name")
	}
}

func TestListRejectsInvalidOrUnboundedCatalog(t *testing.T) {
	for name, found := range map[string][]SkillSummary{
		"invalid row": {{Name: "review", Description: "Review the project changes.", Scope: SkillScope("unknown")}},
		"capacity":    make([]SkillSummary, 2*skills.MaxSkillsPerSource+1),
	} {
		t.Run(name, func(t *testing.T) {
			c := newSkills(t, newScope(t, "", "", testPaths{}), &fakeSkillCatalog{skills: found}, nil, &fakeSkillProposals{}, nil, nil)
			if _, err := c.List(t.Context(), "/repo"); err == nil {
				t.Fatal("List error = nil, want rejected catalog")
			}
		})
	}
}

func TestListEmptyCatalogReturnsNil(t *testing.T) {
	c := newSkills(t, newScope(t, "", "", testPaths{}), &fakeSkillCatalog{}, nil, &fakeSkillProposals{}, nil, nil)
	got, err := c.List(context.Background(), "/repo")
	if err != nil || got != nil {
		t.Fatalf("List = %v, %v; want nil, nil", got, err)
	}
}

func TestListPreservesCatalogFailure(t *testing.T) {
	cause := errors.New("skill directory unavailable")
	c := newSkills(t, newScope(t, "", "", testPaths{}), &fakeSkillCatalog{err: cause}, nil, &fakeSkillProposals{}, nil, nil)
	if _, err := c.List(t.Context(), "/repo"); !errors.Is(err, cause) {
		t.Fatalf("List error = %v, want catalog failure", err)
	}
}

func TestManagedSkillsWithoutCuratorReportUnavailable(t *testing.T) {
	c := newSkills(t, newScope(t, "", "", testPaths{}), &fakeSkillCatalog{}, nil, &fakeSkillProposals{}, nil, nil)
	if _, err := c.Managed(context.Background()); !errors.Is(err, ErrSkillLibraryUnavailable) {
		t.Fatalf("Managed err = %v, want ErrSkillLibraryUnavailable", err)
	}
	if err := c.Archive(context.Background(), "lint"); !errors.Is(err, ErrSkillLibraryUnavailable) {
		t.Fatalf("Archive err = %v, want ErrSkillLibraryUnavailable", err)
	}
	if err := c.Restore(context.Background(), "lint"); !errors.Is(err, ErrSkillLibraryUnavailable) {
		t.Fatalf("Restore err = %v, want ErrSkillLibraryUnavailable", err)
	}
}

func TestSkillMutationsPublishOnlyCommittedFilesystemFacts(t *testing.T) {
	curator := &fakeSkillCurator{}
	proposals := &fakeSkillProposals{}
	watcher := &recordingAuthoredWatcher{}
	observations := newAuthoredWatch(t, newScope(t, "", "", testPaths{}), staticWorkspaceInspector{
		resolved: Resolved{Path: "/repo", ProjectRoot: "/repo"},
	}, watcher)
	observation, err := observations.Watch([]string{"/repo"}, []AuthoredResource{AuthoredSkills}, func(AuthoredResource) {})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = observation.Close() }()
	var notices []invalidation.Notice
	c := newSkills(t, newScope(t, "", "", testPaths{}), &fakeSkillCatalog{}, curator, proposals, observations, func(notice invalidation.Notice) {
		notices = append(notices, notice)
	})

	if archiveErr2 := c.Archive(context.Background(), "lint"); archiveErr2 != nil {
		t.Fatal(archiveErr2)
	}
	if restoreErr := c.Restore(context.Background(), "lint"); restoreErr != nil {
		t.Fatal(restoreErr)
	}
	proposal := skills.Proposal{Scope: skills.ScopeProject, Name: "lint", Description: "Lint the current project before final verification.", Instructions: "Run the linter."}
	ref, err := c.SubmitProposal(context.Background(), "/repo", proposal)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ApproveProposal(context.Background(), "/repo", ref); err != nil {
		t.Fatal(err)
	}
	if err := c.RejectProposal(context.Background(), "/repo", ref); err != nil {
		t.Fatal(err)
	}
	if len(notices) != 5 {
		t.Fatalf("notifications = %d, want 5", len(notices))
	}
	for _, notice := range notices {
		if notice.Resource != invalidation.Skills {
			t.Fatalf("notice = %+v, want Skills", notice)
		}
	}
	if len(watcher.accepted) != 5 {
		t.Fatalf("accepted authored changes = %+v, want 5", watcher.accepted)
	}

	curator.archiveErr = errors.New("disk unavailable")
	curator.archiveIdentities = []string{"/skills/partially-moved/SKILL.md"}
	if err := c.Archive(context.Background(), "lint"); err == nil {
		t.Fatal("Archive error = nil, want failure")
	}
	if len(notices) != 6 || len(watcher.accepted) != 6 {
		t.Fatalf("partially committed mutation = notices %d, accepted %d; want 6, 6", len(notices), len(watcher.accepted))
	}
	curator.archiveIdentities = nil
	proposals.approveErr = errors.New("disk unavailable")
	if err := c.ApproveProposal(context.Background(), "/repo", ref); err == nil {
		t.Fatal("ApproveProposal error = nil, want failure")
	}
	proposals.rejectErr = errors.New("disk unavailable")
	if err := c.RejectProposal(context.Background(), "/repo", ref); err == nil {
		t.Fatal("RejectProposal error = nil, want failure")
	}
	if len(notices) != 6 {
		t.Fatalf("uncommitted failure notifications = %d, want 6", len(notices))
	}
}

type fakeSkillCatalog struct {
	cwd    string
	skills []SkillSummary
	err    error
}

type fakeSkillCurator struct {
	entries           []skills.Entry
	archiveErr        error
	archiveIdentities []string
}

func (f *fakeSkillCurator) List(context.Context) ([]skills.Entry, error) {
	return slices.Clone(f.entries), nil
}
func (f *fakeSkillCurator) Archive(context.Context, string) ([]string, error) {
	if f.archiveErr != nil {
		return f.archiveIdentities, f.archiveErr
	}
	return []string{"/skills/lint/SKILL.md"}, nil
}
func (f *fakeSkillCurator) Restore(context.Context, string) ([]string, error) {
	return []string{"/skills/lint/SKILL.md"}, nil
}

type testPaths struct{}

func (testPaths) ResolveExistingDir(path string) (string, error) { return path, nil }
func (testPaths) ResolveInRoot(_, path string) (string, error)   { return path, nil }
func (testPaths) ResolveExistingInRoot(_, path string) (string, error) {
	return path, nil
}

func (f *fakeSkillCatalog) List(_ context.Context, cwd string) ([]SkillSummary, error) {
	f.cwd = cwd
	return slices.Clone(f.skills), f.err
}

func TestManagedSkillsOwnLifecycleAndNameOrder(t *testing.T) {
	curator := &fakeSkillCurator{entries: []skills.Entry{
		{Name: "omega", Description: "Run the omega workflow.", Lifecycle: skills.Archived},
		{Name: "zeta", Description: "Run the zeta workflow.", Lifecycle: skills.Active},
		{Name: "alpha", Description: "Run the alpha workflow.", Lifecycle: skills.Active},
		{Name: "beta", Description: "Run the beta workflow.", Lifecycle: skills.Archived},
	}}
	c := newSkills(t, newScope(t, "", "", testPaths{}), &fakeSkillCatalog{}, curator, &fakeSkillProposals{}, nil, nil)

	got, err := c.Managed(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[0].Name != "alpha" || got[1].Name != "zeta" || got[2].Name != "beta" || got[3].Name != "omega" {
		t.Fatalf("Managed = %+v, want active alpha/zeta then archived beta/omega", got)
	}
	if curator.entries[0].Name != "omega" {
		t.Fatal("Managed reordered curator-owned storage")
	}
	got[0].Name = "caller edit"
	next, err := c.Managed(t.Context())
	if err != nil || len(next) != 4 || next[0].Name != "alpha" {
		t.Fatalf("Managed after caller reused result = (%+v, %v)", next, err)
	}
}

func TestManagedSkillsRejectDuplicateNameAcrossLifecycles(t *testing.T) {
	curator := &fakeSkillCurator{entries: []skills.Entry{
		{Name: "review", Description: "Review active changes.", Lifecycle: skills.Active},
		{Name: "review", Description: "Review archived changes.", Lifecycle: skills.Archived},
	}}
	c := newSkills(t, newScope(t, "", "", testPaths{}), &fakeSkillCatalog{}, curator, &fakeSkillProposals{}, nil, nil)

	if _, err := c.Managed(t.Context()); err == nil {
		t.Fatal("Managed accepted one Skill name in two lifecycles")
	}
}

func TestManagedSkillsRejectInvalidOrUnboundedCatalog(t *testing.T) {
	for name, entries := range map[string][]skills.Entry{
		"invalid row": {{Name: "review", Description: "Review the project changes.", Lifecycle: skills.Lifecycle("unknown")}},
		"capacity":    make([]skills.Entry, skills.MaxSkillsPerSource+1),
	} {
		t.Run(name, func(t *testing.T) {
			c := newSkills(t, newScope(t, "", "", testPaths{}), &fakeSkillCatalog{}, &fakeSkillCurator{entries: entries}, &fakeSkillProposals{}, nil, nil)
			if _, err := c.Managed(t.Context()); err == nil {
				t.Fatal("Managed error = nil, want rejected catalog")
			}
		})
	}
}
