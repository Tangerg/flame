package coordination

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	domain "github.com/Tangerg/flame/plugins/memory/internal/domain"
	"github.com/Tangerg/flame/plugins/memory/internal/management"
)

func testMemoryItemID(digit byte) domain.ItemID {
	id, err := domain.ParseItemID(domain.ItemIDPrefix + strings.Repeat(
		string(digit),
		domain.MaximumItemIDCharacters-len(domain.ItemIDPrefix),
	))
	if err != nil {
		panic(err)
	}
	return id
}

type fakeStore struct {
	listScope   domain.Scope
	listProject string
	listed      []domain.Item
	updatedAt   time.Time
	content     *string
	pinned      *bool
	decision    domain.ReviewDecision
	err         error
	addChanged  bool
	applied     int
}

func (f *fakeStore) Get(_ context.Context, id domain.ItemID) (domain.Item, bool, error) {
	for _, item := range f.listed {
		if item.ID() == id {
			return item, true, f.err
		}
	}
	return domain.Item{}, false, f.err
}

func TestItemReadFencesItsResolvedTarget(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	item := validMemoryItem(testMemoryItemID('1'), domain.ScopeProject, "/canonical", "project fact", false, now)
	store := &fakeStore{listed: []domain.Item{item}}
	c := newCoordinator(t, Config{Store: store, Roots: rootResolver{root: "/canonical"}})
	if actual, err := c.Item(t.Context(), domain.ScopeProject, "/alias", item.ID().String()); err != nil || actual.ID() != item.ID() {
		t.Fatalf("resolved item: %+v, %v", actual, err)
	}
	if _, err := c.Item(t.Context(), domain.ScopeUser, "", item.ID().String()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("project item escaped into user scope: %v", err)
	}
	c = newCoordinator(t, Config{Store: store, Roots: rootResolver{root: "/other"}})
	if _, err := c.Item(t.Context(), domain.ScopeProject, "/other", item.ID().String()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("item escaped into another workspace: %v", err)
	}
	store.err = errors.New("storage unavailable")
	if _, err := c.Item(t.Context(), domain.ScopeProject, "/canonical", item.ID().String()); !errors.Is(err, store.err) || errors.Is(err, ErrInvalidInput) {
		t.Fatalf("storage failure became an input refusal: %v", err)
	}
}

func validMemoryItem(id domain.ItemID, scope domain.Scope, project, content string, pinned bool, now time.Time) domain.Item {
	item, err := domain.NewUserItem(id, scope, project, content, now)
	if err != nil {
		panic(err)
	}
	change, err := item.Edit(item.Revision(), nil, &pinned, now)
	if err != nil {
		panic(err)
	}
	return change.Item()
}

func (f *fakeStore) List(_ context.Context, scope domain.Scope, project string) ([]domain.Item, error) {
	f.listScope, f.listProject = scope, project
	if f.listed != nil {
		return cloneMemoryItems(f.listed), f.err
	}
	return []domain.Item{validMemoryItem(
		testMemoryItemID('1'), scope, project, "fact", false,
		time.Date(2026, time.September, 4, 8, 0, 0, 0, time.UTC),
	)}, f.err
}

func (f *fakeStore) persist(context.Context) error { f.applied++; return nil }

func (f *fakeStore) PrepareReview(_ context.Context, id domain.ItemID, _ uint64, decision domain.ReviewDecision, now time.Time) (management.Mutation, error) {
	f.decision = decision
	return management.New(validMemoryItem(id, domain.ScopeUser, "", "fact", false, now), f.persist), f.err
}

func (f *fakeStore) PrepareUpdate(_ context.Context, id domain.ItemID, _ uint64, content *string, pinned *bool, now time.Time) (management.Mutation, error) {
	f.content, f.pinned, f.updatedAt = nil, nil, now
	if content != nil {
		f.content = new(*content)
	}
	if pinned != nil {
		f.pinned = new(*pinned)
	}
	text := "fact"
	if content != nil {
		text = *content
	}
	isPinned := false
	if pinned != nil {
		isPinned = *pinned
	}
	return management.New(validMemoryItem(id, domain.ScopeUser, "", text, isPinned, now), f.persist), f.err
}

func (f *fakeStore) PrepareDelete(_ context.Context, id domain.ItemID, _ uint64) (management.Mutation, error) {
	return management.New(validMemoryItem(id, domain.ScopeUser, "", "fact", false, time.Unix(1, 0)), f.persist), f.err
}

func (f *fakeStore) PrepareAdd(_ context.Context, scope domain.Scope, project, content string, now time.Time) (management.Mutation, error) {
	var persist func(context.Context) error
	if f.addChanged {
		persist = f.persist
	}
	return management.New(validMemoryItem(testMemoryItemID('2'), scope, project, content, false, now), persist), f.err
}

type rootResolver struct {
	root string
	err  error
}

func (r rootResolver) ResolveExistingDir(string) (string, error) { return r.root, r.err }

func TestListResolvesProjectAtApplicationBoundary(t *testing.T) {
	store := &fakeStore{}
	c := newCoordinator(t, Config{Store: store, Roots: rootResolver{root: "/canonical/repo"}})

	items, err := c.List(context.Background(), ListInput{Scope: domain.ScopeProject, CWD: "/repo/../repo"})
	if err != nil || len(items.Rows) != 1 {
		t.Fatalf("List = (%+v, %v)", items, err)
	}
	if store.listScope != domain.ScopeProject || store.listProject != "/canonical/repo" {
		t.Fatalf("store target = %v %q", store.listScope, store.listProject)
	}

	if _, err := c.List(context.Background(), ListInput{Scope: domain.ScopeUser, CWD: "/ignored"}); err != nil {
		t.Fatal(err)
	}
	if store.listScope != domain.ScopeUser || store.listProject != "" {
		t.Fatalf("user target = %v %q", store.listScope, store.listProject)
	}
}

func TestListOwnsManagementOrder(t *testing.T) {
	t.Parallel()
	updated := time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)
	item := func(id byte, status domain.Status, pinned bool, updatedAt time.Time) domain.Item {
		origin := domain.OriginAuto
		if status == domain.StatusActive {
			origin = domain.OriginUser
		}
		return mustAgentMemoryItem(domain.ItemSnapshot{Revision: 1,
			ID: testMemoryItemID(id), Scope: domain.ScopeProject, Project: "/repo",
			Content: string(id), Origin: origin, Status: status, Pinned: pinned,
			CreatedAt: updated, UpdatedAt: updatedAt,
		})
	}
	store := &fakeStore{listed: []domain.Item{
		item('6', domain.StatusActive, false, updated.Add(time.Hour)),
		item('1', domain.StatusPending, true, updated),
		item('4', domain.StatusPending, false, updated.Add(time.Hour)),
		item('5', domain.StatusActive, true, updated),
		item('2', domain.StatusPending, true, updated),
		item('0', domain.StatusPending, true, updated.Add(time.Hour)),
	}}
	c := newCoordinator(t, Config{Store: store, Roots: rootResolver{root: "/repo"}})

	items, err := c.List(t.Context(), ListInput{Scope: domain.ScopeProject, CWD: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(items.Rows))
	for _, value := range items.Rows {
		got = append(got, value.ID().String())
	}
	want := []string{
		testMemoryItemID('0').String(),
		testMemoryItemID('2').String(),
		testMemoryItemID('1').String(),
		testMemoryItemID('4').String(),
		testMemoryItemID('5').String(),
		testMemoryItemID('6').String(),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("List order = %v, want %v", got, want)
	}
	if store.listed[0].ID() != testMemoryItemID('6') {
		t.Fatal("List mutated persistence-owned rows")
	}
}

func TestUpdateDelegatesOneAtomicPatchWithApplicationClock(t *testing.T) {
	store := &fakeStore{}
	now := time.Date(2026, 7, 23, 9, 0, 0, 0, time.UTC)
	c := newCoordinator(t, Config{Store: store, Now: func() time.Time { return now }})
	content := "- use table-driven tests"
	pinned := true

	if _, err := c.PrepareUpdate(context.Background(), testMemoryItemID('1').String(), 1, &content, &pinned); err != nil {
		t.Fatal(err)
	}
	if store.content == nil || *store.content != content || store.pinned == nil || *store.pinned != pinned || !store.updatedAt.Equal(now) {
		t.Fatalf("patch = content=%p pinned=%p at=%s", store.content, store.pinned, store.updatedAt)
	}
}

func TestUpdateAllowsCallerReuseAfterReturn(t *testing.T) {
	store := &fakeStore{}
	content, pinned := "original fact", true
	coordinator := newCoordinator(t, Config{Store: store})
	item, err := coordinator.PrepareUpdate(t.Context(), testMemoryItemID('1').String(), 1, &content, &pinned)
	if err != nil {
		t.Fatal(err)
	}
	content, pinned = "caller reuse", false
	if store.content == nil || *store.content != "original fact" || store.pinned == nil || !*store.pinned {
		t.Fatalf("retained patch = content=%v pinned=%v", store.content, store.pinned)
	}
	if item.Item().Content() != "original fact" || !item.Item().Pinned() {
		t.Fatalf("acknowledged item = %+v, want original patch", item)
	}
}

func TestReviewAcceptsDecisionNotTargetState(t *testing.T) {
	store := &fakeStore{}
	c := newCoordinator(t, Config{Store: store})
	if _, err := c.PrepareReview(t.Context(), testMemoryItemID('1').String(), 1, domain.ReviewApprove); err != nil {
		t.Fatal(err)
	}
	if store.decision != domain.ReviewApprove {
		t.Fatalf("decision = %q, want approve", store.decision)
	}
	if _, err := c.PrepareReview(t.Context(), testMemoryItemID('1').String(), 1, domain.ReviewDecision("active")); err == nil {
		t.Fatal("target status was accepted as a review decision")
	}
}

func TestMutationRejectsNonCanonicalItemIdentityBeforeStore(t *testing.T) {
	wantStoreErr := errors.New("store must not be reached")
	c := newCoordinator(t, Config{Store: &fakeStore{err: wantStoreErr}})

	if _, err := c.PrepareReview(t.Context(), "mem_1", 1, domain.ReviewApprove); err == nil || errors.Is(err, wantStoreErr) {
		t.Fatalf("Review error = %v, want invalid identity before store", err)
	}
	if _, err := c.PrepareUpdate(t.Context(), "mem_1", 1, nil, nil); err == nil || errors.Is(err, wantStoreErr) {
		t.Fatalf("Update error = %v, want invalid identity before store", err)
	}
	if _, err := c.PrepareDelete(t.Context(), "mem_1", 1); err == nil || errors.Is(err, wantStoreErr) {
		t.Fatalf("Delete error = %v, want invalid identity before store", err)
	}
}

func TestUnknownScopeFailsBeforeRootResolution(t *testing.T) {
	store := &fakeStore{}
	c := newCoordinator(t, Config{Store: store, Roots: rootResolver{root: "/canonical/repo"}})
	if _, err := c.List(t.Context(), ListInput{Scope: domain.Scope("unknown"), CWD: "/repo"}); err == nil {
		t.Fatal("unknown scope was accepted")
	}
}

func TestCoordinatorRequiresCompleteDependencies(t *testing.T) {
	var missingStore *fakeStore
	for _, cfg := range []Config{
		{},
		{Store: &fakeStore{}},
		{Roots: rootResolver{root: "/repo"}},
		{Store: missingStore, Roots: rootResolver{root: "/repo"}},
	} {
		if c, err := New(cfg); err == nil || c != nil {
			t.Fatalf("New returned (%v, %v) for incomplete dependencies", c, err)
		}
	}
}

func TestPreparationRetainsChangesWithoutApplyingThem(t *testing.T) {
	store := &fakeStore{addChanged: true}
	c := newCoordinator(t, Config{Store: store, Roots: rootResolver{root: "/repo"}})
	preparations := []func() (management.Mutation, error){
		func() (management.Mutation, error) {
			return c.PrepareReview(t.Context(), testMemoryItemID('1').String(), 1, domain.ReviewApprove)
		},
		func() (management.Mutation, error) {
			return c.PrepareUpdate(t.Context(), testMemoryItemID('1').String(), 1, new("updated"), new(true))
		},
		func() (management.Mutation, error) {
			return c.PrepareDelete(t.Context(), testMemoryItemID('1').String(), 1)
		},
		func() (management.Mutation, error) {
			return c.PrepareAdd(t.Context(), domain.ScopeProject, "/repo", "new")
		},
	}
	for _, prepare := range preparations {
		before := store.applied
		mutation, err := prepare()
		if err != nil || !mutation.Changed() || store.applied != before {
			t.Fatalf("preparation changed storage: changed=%t, writes=%d, err=%v", mutation.Changed(), store.applied-before, err)
		}
		if err := mutation.Apply(t.Context()); err != nil || store.applied != before+1 {
			t.Fatalf("mutation failed to apply its decision once: writes=%d, err=%v", store.applied-before, err)
		}
	}
	store.addChanged = false
	mutation, err := c.PrepareAdd(t.Context(), domain.ScopeProject, "/repo", "new")
	if err != nil || mutation.Changed() {
		t.Fatalf("duplicate changed the target: %v, %v", mutation.Changed(), err)
	}
	if err := mutation.Apply(t.Context()); err != nil || store.applied != len(preparations) {
		t.Fatalf("duplicate applied a write: %d, %v", store.applied, err)
	}
}

func TestFailedPreparationCannotApplyStorage(t *testing.T) {
	wantErr := errors.New("store unavailable")
	store := &fakeStore{err: wantErr}
	c := newCoordinator(t, Config{Store: store})
	if _, err := c.PrepareUpdate(t.Context(), testMemoryItemID('1').String(), 1, nil, nil); !errors.Is(err, wantErr) {
		t.Fatalf("Update error = %v, want %v", err, wantErr)
	}
	if store.applied != 0 {
		t.Fatalf("failed preparation applied writes: %d", store.applied)
	}
}

func newCoordinator(t *testing.T, cfg Config) *Coordinator {
	t.Helper()
	if cfg.Roots == nil {
		cfg.Roots = rootResolver{root: "/repo"}
	}
	coordinator, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return coordinator
}

func cloneMemoryItems(items []domain.Item) []domain.Item {
	return slices.Clone(items)
}
