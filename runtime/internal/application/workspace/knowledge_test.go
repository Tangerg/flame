package workspace

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/knowledge"
)

func TestNewKnowledgeRequiresCompleteDependencies(t *testing.T) {
	scope := newScope(t, "", "", testPaths{})
	for _, test := range []struct {
		name      string
		scope     *Scope
		inspector KnowledgeWorkspaceInspector
		store     KnowledgeStore
	}{
		{name: "scope", inspector: knowledgeInspector{}, store: &fakeKnowledgeStore{}},
		{name: "inspector", scope: scope, store: &fakeKnowledgeStore{}},
		{name: "typed nil inspector", scope: scope, inspector: (*knowledgeInspector)(nil), store: &fakeKnowledgeStore{}},
		{name: "store", scope: scope, inspector: knowledgeInspector{}},
		{name: "typed nil store", scope: scope, inspector: knowledgeInspector{}, store: (*fakeKnowledgeStore)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if knowledge, err := NewKnowledge(test.scope, test.inspector, test.store, nil, nil); err == nil || knowledge != nil {
				t.Fatalf("NewKnowledge = (%v, %v), want incomplete construction rejected", knowledge, err)
			}
		})
	}
}

func newKnowledge(t *testing.T, scope *Scope, inspector KnowledgeWorkspaceInspector, store KnowledgeStore, observations *AuthoredWatch, publish invalidation.Publish) *Knowledge {
	t.Helper()
	knowledge, err := NewKnowledge(scope, inspector, store, observations, publish)
	if err != nil {
		t.Fatal(err)
	}
	return knowledge
}

func TestRuntimeKnowledgePorts(t *testing.T) {
	ctx := context.Background()
	var notices []invalidation.Notice
	store := &fakeKnowledgeStore{
		entries: []knowledge.Entry{
			{Scope: knowledge.ScopeHome, Path: "/home/.flame/FLAME.md", Content: "prefs", Revision: "rev-home"},
			{Scope: knowledge.ScopeCWD, Path: "/repo/work/FLAME.md", Revision: "rev-cwd"},
		},
		entry: knowledge.Entry{
			Scope: knowledge.ScopeProjectRoot, Path: "/repo/FLAME.md",
			Content: "project notes", Revision: "rev-1",
		},
	}
	c := newKnowledge(t,
		newScope(t, "", "", testPaths{}),
		knowledgeInspector{resolved: Resolved{Path: "/repo/work", ProjectRoot: "/repo"}},
		store, nil, func(notice invalidation.Notice) { notices = append(notices, notice) },
	)

	entries, err := c.Entries(ctx, "/repo/work")
	if err != nil {
		t.Fatalf("Entries err = %v", err)
	}
	if len(entries) != 2 || entries[0].Content != "prefs" || store.listCWD != "/repo/work" || store.listProjectRoot != "/repo" {
		t.Fatalf("Entries = %+v, cwd = %q, projectRoot = %q", entries, store.listCWD, store.listProjectRoot)
	}
	entries[0].Content = "caller reuse"
	again, err := c.Entries(ctx, "/repo/work")
	if err != nil || len(again) != 2 || again[0].Content != "prefs" {
		t.Fatalf("Entries after caller reuse = (%+v, %v)", again, err)
	}

	got, err := c.Read(ctx, knowledge.ScopeProjectRoot, "/repo/work")
	if err != nil {
		t.Fatalf("Read err = %v", err)
	}
	if got.Content != "project notes" || got.Revision != "rev-1" || store.getScope != knowledge.ScopeProjectRoot || store.getCWD != "/repo" {
		t.Fatalf("Read = %+v, scope = %v, cwd = %q", got, store.getScope, store.getCWD)
	}

	written, err := c.Update(ctx, knowledge.ScopeHome, "", "rev-1", "global prefs")
	if err != nil {
		t.Fatalf("Update err = %v", err)
	}
	if written.Content != "global prefs" || store.updateScope != knowledge.ScopeHome || store.updateCWD != "" || store.updateRevision != "rev-1" || store.updateContent != "global prefs" {
		t.Fatalf("Update scope = %v, cwd = %q, content = %q", store.updateScope, store.updateCWD, store.updateContent)
	}
	if !reflect.DeepEqual(notices, []invalidation.Notice{{Resource: invalidation.Knowledge}}) {
		t.Fatalf("invalidations = %+v, want knowledge", notices)
	}
}

func TestRuntimeKnowledgeRejectsUnknownScopeBeforeDispatch(t *testing.T) {
	store := &fakeKnowledgeStore{}
	c := newKnowledge(t, newScope(t, "", "", testPaths{}), knowledgeInspector{}, store, nil, nil)
	unknown := knowledge.Scope("workspace")

	if _, err := c.Read(t.Context(), unknown, "/repo"); err == nil {
		t.Fatal("Read accepted an unknown scope")
	}
	if _, err := c.Update(t.Context(), unknown, "/repo", "rev-1", "notes"); err == nil {
		t.Fatal("Update accepted an unknown scope")
	}
	if store.getScope != "" || store.updateScope != "" {
		t.Fatalf("invalid scope reached store: get=%q update=%q", store.getScope, store.updateScope)
	}
}

func TestRuntimeKnowledgeRejectsOversizedContentBeforeStore(t *testing.T) {
	store := &fakeKnowledgeStore{}
	var notices []invalidation.Notice
	c := newKnowledge(t,
		newScope(t, "", "", testPaths{}),
		knowledgeInspector{},
		store,
		nil,
		func(notice invalidation.Notice) { notices = append(notices, notice) },
	)

	content := strings.Repeat("x", int(knowledge.MaxDocumentBytes)+1)
	if _, err := c.Update(t.Context(), knowledge.ScopeHome, "", "rev-1", content); !errors.Is(err, knowledge.ErrDocumentTooLarge) {
		t.Fatalf("Update error = %v, want ErrDocumentTooLarge", err)
	}
	if store.updateContent != "" {
		t.Fatal("oversized knowledge content reached the persistence port")
	}
	if len(notices) != 0 {
		t.Fatalf("oversized update published invalidations: %+v", notices)
	}
}

func TestRuntimeKnowledgeRejectsInvalidDurableMaterial(t *testing.T) {
	validHome := knowledge.Entry{
		Scope: knowledge.ScopeHome, Path: "/home/.flame/FLAME.md", Revision: "rev-home",
	}
	validCWD := knowledge.Entry{
		Scope: knowledge.ScopeCWD, Path: "/repo/work/FLAME.md", Revision: "rev-cwd",
	}
	store := &fakeKnowledgeStore{entries: []knowledge.Entry{validCWD, validHome}}
	var notices []invalidation.Notice
	curation := newKnowledge(t,
		newScope(t, "", "", testPaths{}),
		knowledgeInspector{resolved: Resolved{Path: "/repo/work", ProjectRoot: "/repo"}},
		store, nil, func(notice invalidation.Notice) { notices = append(notices, notice) },
	)

	if _, err := curation.Entries(t.Context(), "/repo/work"); err == nil {
		t.Fatal("out-of-order knowledge cascade was accepted")
	}
	store.entry = knowledge.Entry{
		Scope: knowledge.ScopeHome, Path: validHome.Path, Revision: validHome.Revision,
	}
	if _, err := curation.Read(t.Context(), knowledge.ScopeCWD, "/repo/work"); err == nil {
		t.Fatal("foreign knowledge entry was accepted")
	}

	store.updateEntry = knowledge.Entry{
		Scope: knowledge.ScopeHome, Path: validHome.Path, Content: "different", Revision: "rev-2",
	}
	if _, err := curation.Update(t.Context(), knowledge.ScopeHome, "", "rev-1", "requested"); err == nil {
		t.Fatal("incorrect knowledge update acknowledgement was accepted")
	}
	if len(notices) != 0 {
		t.Fatalf("invalid acknowledgement published invalidations: %+v", notices)
	}
}

func TestRuntimeKnowledgeMapsInfraContainmentWithoutLeakingFilesystemMechanics(t *testing.T) {
	store := &fakeKnowledgeStore{err: knowledge.ErrPathOutsideScope}
	c := newKnowledge(t,
		newScope(t, "", "", testPaths{}),
		knowledgeInspector{resolved: Resolved{Path: "/repo", ProjectRoot: "/repo"}},
		store, nil, nil,
	)

	if _, err := c.Entries(t.Context(), "/repo"); !errors.Is(err, ErrPathOutsideRoot) {
		t.Fatalf("Entries error = %v, want ErrPathOutsideRoot", err)
	}
	if _, err := c.Read(t.Context(), knowledge.ScopeHome, ""); !errors.Is(err, ErrPathOutsideRoot) {
		t.Fatalf("Read error = %v, want ErrPathOutsideRoot", err)
	}
	if _, err := c.Update(t.Context(), knowledge.ScopeHome, "", "rev-1", "notes"); !errors.Is(err, ErrPathOutsideRoot) {
		t.Fatalf("Update error = %v, want ErrPathOutsideRoot", err)
	}
}

type fakeKnowledgeStore struct {
	entries     []knowledge.Entry
	entry       knowledge.Entry
	updateEntry knowledge.Entry
	err         error

	listCWD         string
	listProjectRoot string

	getScope knowledge.Scope
	getCWD   string

	updateScope    knowledge.Scope
	updateCWD      string
	updateRevision string
	updateContent  string
}

func (f *fakeKnowledgeStore) List(_ context.Context, cwd, projectRoot string) ([]knowledge.Entry, error) {
	f.listCWD = cwd
	f.listProjectRoot = projectRoot
	return slices.Clone(f.entries), f.err
}

func (f *fakeKnowledgeStore) Get(_ context.Context, scope knowledge.Scope, cwd string) (knowledge.Entry, error) {
	f.getScope = scope
	f.getCWD = cwd
	return f.entry, f.err
}

func (f *fakeKnowledgeStore) Update(_ context.Context, cwd string, replacement knowledge.Replacement) (knowledge.Entry, error) {
	f.updateScope = replacement.Scope()
	f.updateCWD = cwd
	f.updateRevision = replacement.ExpectedRevision()
	f.updateContent = replacement.Content()
	if f.err != nil {
		return knowledge.Entry{}, f.err
	}
	if f.updateEntry != (knowledge.Entry{}) {
		return f.updateEntry, nil
	}
	return knowledge.Entry{
		Scope: replacement.Scope(), Path: "/home/.flame/FLAME.md", Content: replacement.Content(), Revision: "rev-2",
	}, nil
}

type knowledgeInspector struct {
	resolved Resolved
	err      error
}

func (k knowledgeInspector) Inspect(string) (Resolved, error) {
	return k.resolved, k.err
}
