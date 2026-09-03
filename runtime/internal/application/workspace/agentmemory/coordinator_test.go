package agentmemory

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	domain "github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
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
}

func (f *fakeStore) List(_ context.Context, scope domain.Scope, project string) ([]domain.Item, error) {
	f.listScope, f.listProject = scope, project
	if f.listed != nil {
		return f.listed, f.err
	}
	return []domain.Item{{ID: testMemoryItemID('1'), Scope: scope, Project: project}}, nil
}

func (f *fakeStore) Review(_ context.Context, _ domain.ItemID, decision domain.ReviewDecision, _ time.Time) error {
	f.decision = decision
	return f.err
}

func (f *fakeStore) Update(_ context.Context, _ domain.ItemID, content *string, pinned *bool, now time.Time) (domain.Item, error) {
	f.content, f.pinned, f.updatedAt = content, pinned, now
	return domain.Item{ID: testMemoryItemID('1')}, f.err
}

func (f *fakeStore) Delete(context.Context, domain.ItemID) error { return f.err }

func (f *fakeStore) Add(context.Context, domain.Scope, string, string, time.Time) (domain.Item, bool, error) {
	return domain.Item{}, f.addChanged, f.err
}

type rootResolver struct {
	root string
	err  error
}

func (r rootResolver) ResolveRoot(string) (string, error) { return r.root, r.err }

func TestListResolvesProjectAtApplicationBoundary(t *testing.T) {
	store := &fakeStore{}
	c := New(Config{Store: store, Roots: rootResolver{root: "/canonical/repo"}})

	items, err := c.List(context.Background(), domain.ScopeProject, "/repo/../repo")
	if err != nil || len(items) != 1 {
		t.Fatalf("List = (%+v, %v)", items, err)
	}
	if store.listScope != domain.ScopeProject || store.listProject != "/canonical/repo" {
		t.Fatalf("store target = %v %q", store.listScope, store.listProject)
	}

	if _, err := c.List(context.Background(), domain.ScopeUser, "/ignored"); err != nil {
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
		return domain.Item{
			ID: testMemoryItemID(id), Scope: domain.ScopeProject, Project: "/repo",
			Content: string(id), Origin: origin, Status: status, Pinned: pinned,
			CreatedAt: updated, UpdatedAt: updatedAt,
		}
	}
	store := &fakeStore{listed: []domain.Item{
		item('6', domain.StatusActive, false, updated.Add(time.Hour)),
		item('1', domain.StatusPending, true, updated),
		item('4', domain.StatusPending, false, updated.Add(time.Hour)),
		item('5', domain.StatusActive, true, updated),
		item('2', domain.StatusPending, true, updated),
		item('0', domain.StatusPending, true, updated.Add(time.Hour)),
	}}
	c := New(Config{Store: store, Roots: rootResolver{root: "/repo"}})

	items, err := c.List(t.Context(), domain.ScopeProject, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(items))
	for _, value := range items {
		got = append(got, value.ID.String())
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
	if store.listed[0].ID != testMemoryItemID('6') {
		t.Fatal("List mutated persistence-owned rows")
	}
}

func TestUpdateDelegatesOneAtomicPatchWithApplicationClock(t *testing.T) {
	store := &fakeStore{}
	now := time.Date(2026, 7, 23, 9, 0, 0, 0, time.UTC)
	c := New(Config{Store: store, Now: func() time.Time { return now }})
	content := "- use table-driven tests"
	pinned := true

	if _, err := c.Update(context.Background(), testMemoryItemID('1').String(), &content, &pinned); err != nil {
		t.Fatal(err)
	}
	if store.content != &content || store.pinned != &pinned || !store.updatedAt.Equal(now) {
		t.Fatalf("patch = content=%p pinned=%p at=%s", store.content, store.pinned, store.updatedAt)
	}
}

func TestReviewAcceptsDecisionNotTargetState(t *testing.T) {
	store := &fakeStore{}
	c := New(Config{Store: store})
	if err := c.Review(t.Context(), testMemoryItemID('1').String(), domain.ReviewApprove); err != nil {
		t.Fatal(err)
	}
	if store.decision != domain.ReviewApprove {
		t.Fatalf("decision = %q, want approve", store.decision)
	}
	if err := c.Review(t.Context(), testMemoryItemID('1').String(), domain.ReviewDecision("active")); err == nil {
		t.Fatal("target status was accepted as a review decision")
	}
}

func TestMutationRejectsNonCanonicalItemIdentityBeforeStore(t *testing.T) {
	wantStoreErr := errors.New("store must not be reached")
	c := New(Config{Store: &fakeStore{err: wantStoreErr}})

	if err := c.Review(t.Context(), "mem_1", domain.ReviewApprove); !errors.Is(err, domain.ErrInvalidItemID) {
		t.Fatalf("Review error = %v, want ErrInvalidItemID", err)
	}
	if _, err := c.Update(t.Context(), "mem_1", nil, nil); !errors.Is(err, domain.ErrInvalidItemID) {
		t.Fatalf("Update error = %v, want ErrInvalidItemID", err)
	}
	if err := c.Delete(t.Context(), "mem_1"); !errors.Is(err, domain.ErrInvalidItemID) {
		t.Fatalf("Delete error = %v, want ErrInvalidItemID", err)
	}
}

func TestUnknownScopeFailsBeforeRootResolution(t *testing.T) {
	store := &fakeStore{}
	c := New(Config{Store: store, Roots: rootResolver{root: "/canonical/repo"}})
	if _, err := c.List(t.Context(), domain.Scope("unknown"), "/repo"); err == nil {
		t.Fatal("unknown scope was accepted")
	}
}

func TestDisabledCoordinatorFailsExplicitly(t *testing.T) {
	c := New(Config{})
	if _, err := c.List(context.Background(), domain.ScopeProject, "/repo"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("List error = %v, want ErrUnavailable", err)
	}
}

func TestCommittedAgentMemoryMutationsPublishInvalidations(t *testing.T) {
	var notices []invalidation.Notice
	store := &fakeStore{addChanged: true}
	c := New(Config{
		Store: store,
		Roots: rootResolver{root: "/repo"},
		Invalidations: func(notice invalidation.Notice) {
			notices = append(notices, notice)
		},
	})
	content := "updated"
	pinned := true
	if err := c.Review(t.Context(), testMemoryItemID('1').String(), domain.ReviewApprove); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Update(t.Context(), testMemoryItemID('1').String(), &content, &pinned); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(t.Context(), testMemoryItemID('1').String()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Add(t.Context(), domain.ScopeProject, "/repo", "new"); err != nil {
		t.Fatal(err)
	}
	if len(notices) != 4 {
		t.Fatalf("notices = %+v, want four", notices)
	}
	store.addChanged = false
	if _, err := c.Add(t.Context(), domain.ScopeProject, "/repo", "new"); err != nil {
		t.Fatal(err)
	}
	if len(notices) != 4 {
		t.Fatalf("duplicate Add published notices = %+v", notices)
	}
	for _, notice := range notices {
		if notice.Resource != invalidation.AgentMemory {
			t.Fatalf("notice = %+v, want agent memory", notice)
		}
	}
}

func TestFailedAgentMemoryMutationDoesNotPublishInvalidation(t *testing.T) {
	wantErr := errors.New("store unavailable")
	var notices []invalidation.Notice
	c := New(Config{
		Store: &fakeStore{err: wantErr},
		Invalidations: func(notice invalidation.Notice) {
			notices = append(notices, notice)
		},
	})
	if _, err := c.Update(t.Context(), testMemoryItemID('1').String(), nil, nil); !errors.Is(err, wantErr) {
		t.Fatalf("Update error = %v, want %v", err, wantErr)
	}
	if len(notices) != 0 {
		t.Fatalf("failed mutation published %+v", notices)
	}
}
