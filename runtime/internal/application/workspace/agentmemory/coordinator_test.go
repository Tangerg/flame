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
	listScope    domain.Scope
	listProject  string
	listed       []domain.Item
	updatedAt    time.Time
	content      *string
	pinned       *bool
	decision     domain.ReviewDecision
	err          error
	addChanged   bool
	updateResult *domain.Item
	addResult    *domain.Item
}

func validMemoryItem(id domain.ItemID, scope domain.Scope, project, content string, pinned bool, now time.Time) domain.Item {
	item, err := domain.NewUserItem(id, scope, project, content, now)
	if err != nil {
		panic(err)
	}
	item.Pinned = pinned
	return item
}

func (f *fakeStore) List(_ context.Context, scope domain.Scope, project string) ([]domain.Item, error) {
	f.listScope, f.listProject = scope, project
	if f.listed != nil {
		return f.listed, f.err
	}
	return []domain.Item{validMemoryItem(
		testMemoryItemID('1'), scope, project, "fact", false,
		time.Date(2026, time.September, 4, 8, 0, 0, 0, time.UTC),
	)}, f.err
}

func (f *fakeStore) Review(_ context.Context, _ domain.ItemID, decision domain.ReviewDecision, _ time.Time) error {
	f.decision = decision
	return f.err
}

func (f *fakeStore) Update(_ context.Context, id domain.ItemID, content *string, pinned *bool, now time.Time) (domain.Item, error) {
	f.content, f.pinned, f.updatedAt = content, pinned, now
	if f.updateResult != nil {
		return *f.updateResult, f.err
	}
	text := "fact"
	if content != nil {
		text = *content
	}
	isPinned := false
	if pinned != nil {
		isPinned = *pinned
	}
	return validMemoryItem(id, domain.ScopeUser, "", text, isPinned, now), f.err
}

func (f *fakeStore) Delete(context.Context, domain.ItemID) error { return f.err }

func (f *fakeStore) Add(_ context.Context, scope domain.Scope, project, content string, now time.Time) (domain.Item, bool, error) {
	if f.addResult != nil {
		return *f.addResult, f.addChanged, f.err
	}
	return validMemoryItem(testMemoryItemID('2'), scope, project, content, false, now), f.addChanged, f.err
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

func TestListRejectsCorruptManagementCatalog(t *testing.T) {
	now := time.Date(2026, time.September, 4, 8, 0, 0, 0, time.UTC)
	valid := validMemoryItem(testMemoryItemID('1'), domain.ScopeProject, "/repo", "fact", false, now)
	foreign := validMemoryItem(testMemoryItemID('2'), domain.ScopeProject, "/other", "foreign", false, now)
	rejected := valid
	rejected.Origin = domain.OriginAuto
	rejected.Status = domain.StatusRejected
	invalid := valid
	invalid.Content = " invalid "
	for _, test := range []struct {
		name  string
		items []domain.Item
	}{
		{name: "invalid item", items: []domain.Item{invalid}},
		{name: "foreign target", items: []domain.Item{foreign}},
		{name: "hidden tombstone", items: []domain.Item{rejected}},
		{name: "duplicate identity", items: []domain.Item{valid, valid}},
		{name: "duplicate content", items: []domain.Item{
			valid,
			validMemoryItem(testMemoryItemID('3'), domain.ScopeProject, "/repo", "fact", false, now),
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			coordinator := New(Config{
				Store: &fakeStore{listed: test.items}, Roots: rootResolver{root: "/repo"},
			})
			if _, err := coordinator.List(t.Context(), domain.ScopeProject, "/repo"); err == nil {
				t.Fatal("corrupt management catalog was accepted")
			}
		})
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
	if store.content == nil || *store.content != content || store.pinned == nil || *store.pinned != pinned || !store.updatedAt.Equal(now) {
		t.Fatalf("patch = content=%p pinned=%p at=%s", store.content, store.pinned, store.updatedAt)
	}
}

func TestUpdateSnapshotsPatchBeforeClockCallback(t *testing.T) {
	store := &fakeStore{}
	content := "original fact"
	pinned := true
	now := time.Date(2026, 7, 23, 9, 0, 0, 0, time.UTC)
	coordinator := New(Config{Store: store, Now: func() time.Time {
		content = "clock mutation"
		pinned = false
		return now
	}})

	item, err := coordinator.Update(t.Context(), testMemoryItemID('1').String(), &content, &pinned)
	if err != nil {
		t.Fatal(err)
	}
	if store.content == nil || *store.content != "original fact" || store.pinned == nil || !*store.pinned {
		t.Fatalf("stored patch = content=%v pinned=%v", store.content, store.pinned)
	}
	if item.Content != "original fact" || !item.Pinned {
		t.Fatalf("acknowledged item = %+v, want original patch", item)
	}
}

func TestMutationRejectsInvalidAcknowledgements(t *testing.T) {
	now := time.Date(2026, time.September, 4, 8, 0, 0, 0, time.UTC)
	requestedID := testMemoryItemID('1')
	wrongID := validMemoryItem(testMemoryItemID('2'), domain.ScopeUser, "", "fact", false, now)
	coordinator := New(Config{Store: &fakeStore{updateResult: &wrongID}, Now: func() time.Time { return now }})
	if _, err := coordinator.Update(t.Context(), requestedID.String(), nil, nil); err == nil {
		t.Fatal("mismatched Update acknowledgement was accepted")
	}
	wrongContent := validMemoryItem(requestedID, domain.ScopeUser, "", "old fact", false, now)
	coordinator = New(Config{Store: &fakeStore{updateResult: &wrongContent}, Now: func() time.Time { return now }})
	content := "new fact"
	if _, err := coordinator.Update(t.Context(), requestedID.String(), &content, nil); err == nil {
		t.Fatal("stale Update content acknowledgement was accepted")
	}

	foreign := validMemoryItem(testMemoryItemID('3'), domain.ScopeProject, "/other", "fact", false, now)
	coordinator = New(Config{
		Store: &fakeStore{addResult: &foreign}, Roots: rootResolver{root: "/repo"},
		Now: func() time.Time { return now },
	})
	if _, err := coordinator.Add(t.Context(), domain.ScopeProject, "/repo", "fact"); err == nil {
		t.Fatal("foreign Add acknowledgement was accepted")
	}
	wrongContent = validMemoryItem(testMemoryItemID('4'), domain.ScopeProject, "/repo", "other fact", false, now)
	coordinator = New(Config{
		Store: &fakeStore{addResult: &wrongContent}, Roots: rootResolver{root: "/repo"},
		Now: func() time.Time { return now },
	})
	if _, err := coordinator.Add(t.Context(), domain.ScopeProject, "/repo", "fact"); err == nil {
		t.Fatal("stale Add content acknowledgement was accepted")
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
