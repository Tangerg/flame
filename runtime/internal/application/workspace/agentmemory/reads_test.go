package agentmemory

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	domain "github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
)

func TestManagementContinuationSurvivesDeletedAnchorAndEarlierInsertion(t *testing.T) {
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	item := func(id byte) domain.Item {
		return validMemoryItem(testMemoryItemID(id), domain.ScopeUser, "", string(id), false, now)
	}
	store := &fakeStore{listed: []domain.Item{item('1'), item('2'), item('3')}}
	c := newCoordinator(t, Config{Store: store})
	limit, err := pagination.NewLimit(1)
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.List(t.Context(), ListInput{Scope: domain.ScopeUser, Limit: limit})
	if err != nil || len(first.Rows) != 1 || first.Rows[0].ID() != testMemoryItemID('3') || first.NextCursor == "" {
		t.Fatalf("first = %+v, %v", first, err)
	}
	store.listed = []domain.Item{item('4'), item('2'), item('1')}
	second, err := c.List(t.Context(), ListInput{Scope: domain.ScopeUser, Cursor: first.NextCursor})
	if err != nil || len(second.Rows) != 2 || second.Rows[0].ID() != testMemoryItemID('2') || second.Rows[1].ID() != testMemoryItemID('1') || second.NextCursor != "" {
		t.Fatalf("continuation = %+v, %v", second, err)
	}
	for _, input := range []ListInput{
		{Scope: domain.ScopeProject, CWD: "/other", Cursor: first.NextCursor},
		{Scope: domain.ScopeUser, Cursor: "malformed"},
	} {
		if _, err := c.List(t.Context(), input); !errors.Is(err, pagination.ErrInvalidCursor) {
			t.Fatalf("invalid continuation = %v", err)
		}
	}
}

func TestManagementRefusesMalformedSortPositions(t *testing.T) {
	c := newCoordinator(t, Config{Store: &fakeStore{}})
	for _, key := range [][]string{{"pending"}, {"unknown", "true", "2026-10-10T00:00:00Z", testMemoryItemID('1').String()}, {"active", "yes", "2026-10-10T00:00:00Z", testMemoryItemID('1').String()}, {"active", "false", "not-a-time", testMemoryItemID('1').String()}, {"active", "false", "2026-10-10T00:00:00Z", "bad-id"}} {
		cursor, err := pagination.Encode(managementPageNamespace, []string{string(domain.ScopeUser), ""}, key)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.List(t.Context(), ListInput{Scope: domain.ScopeUser, Cursor: cursor}); !errors.Is(err, pagination.ErrInvalidCursor) {
			t.Fatalf("malformed key %v: %v", key, err)
		}
	}
}

func TestManagementPageBoundAndReadFailure(t *testing.T) {
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{listed: make([]domain.Item, managementPageLimit+1)}
	for i := range store.listed {
		id, err := domain.ParseItemID(fmt.Sprintf("mem_%032x", i+1))
		if err != nil {
			t.Fatal(err)
		}
		store.listed[i] = validMemoryItem(id, domain.ScopeUser, "", fmt.Sprint(i), false, now)
	}
	c := newCoordinator(t, Config{Store: store})
	page, err := c.List(t.Context(), ListInput{Scope: domain.ScopeUser})
	if err != nil || len(page.Rows) != managementPageLimit || page.NextCursor == "" {
		t.Fatalf("bounded page = %+v, %v", page, err)
	}
	store.err = errors.New("storage unavailable")
	if _, err := c.List(t.Context(), ListInput{Scope: domain.ScopeUser}); !errors.Is(err, store.err) {
		t.Fatalf("read failure = %v", err)
	}
	store.err = nil
	store.listed = []domain.Item{}
	page, err = c.List(t.Context(), ListInput{Scope: domain.ScopeUser})
	if err != nil || len(page.Rows) != 0 || page.NextCursor != "" {
		t.Fatalf("empty = %+v, %v", page, err)
	}
}
