package reviews

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func openTestStore(t *testing.T, directory string) *Store {
	t.Helper()
	store, err := OpenStore(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return store
}

func TestUpdateReceiptSurvivesRestartAndRejectsIdentityReuse(t *testing.T) {
	directory := t.TempDir()
	first, err := OpenStore(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	input := Update{ID: ExampleReviewID, ExpectedRevision: 1, Status: Resolved}
	accepted, err := first.Update(t.Context(), "original-call", input)
	if err != nil || accepted.Type != Updated || accepted.Review.Revision != 2 {
		t.Fatalf("update = %+v, %v", accepted, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	restarted := openTestStore(t, directory)
	replayed, err := restarted.Update(t.Context(), "original-call", input)
	if err != nil || !reflect.DeepEqual(replayed, accepted) {
		t.Fatalf("replay = %+v, %v, want %+v", replayed, err, accepted)
	}
	input.Status = Open
	conflicting, err := restarted.Update(t.Context(), "original-call", input)
	if err != nil || conflicting.Type != IdentityConflict {
		t.Fatalf("identity reuse = %+v, %v", conflicting, err)
	}
	input.Status = Resolved
	stale, err := restarted.Update(t.Context(), "another-call", input)
	if err != nil || stale.Type != Conflict || stale.Review.Revision != 2 {
		t.Fatalf("stale update = %+v, %v", stale, err)
	}
	current, err := restarted.List(t.Context())
	if err != nil || len(current) != 1 || !reflect.DeepEqual(current[0], *accepted.Review) {
		t.Fatalf("current = %+v, %v", current, err)
	}
}

func TestReceiptFailureRollsBackBusinessTransition(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	if _, err := store.db.ExecContext(t.Context(), `CREATE TRIGGER reject_receipt BEFORE INSERT ON invocations BEGIN SELECT RAISE(ABORT, 'receipt rejected'); END`); err != nil {
		t.Fatal(err)
	}
	input := Update{ID: ExampleReviewID, ExpectedRevision: 1, Status: Resolved}
	if _, err := store.Update(t.Context(), "original-call", input); err == nil {
		t.Fatal("receipt failure became success")
	}
	current, err := store.List(t.Context())
	if err != nil || current[0].Revision != 1 || current[0].Status != Open {
		t.Fatalf("failed commit advanced review: %+v, %v", current, err)
	}
	if _, err := store.db.ExecContext(t.Context(), `DROP TRIGGER reject_receipt`); err != nil {
		t.Fatal(err)
	}
	result, err := store.Update(t.Context(), "original-call", input)
	if err != nil || result.Type != Updated || result.Review.Revision != 2 {
		t.Fatalf("original call = %+v, %v", result, err)
	}
}

func TestConcurrentDuplicateHasOneBusinessTransition(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			result, err := store.Update(t.Context(), "same-call", Update{ID: ExampleReviewID, ExpectedRevision: 1, Status: Resolved})
			if err != nil || result.Type != Updated || result.Review.Revision != 2 {
				t.Errorf("duplicate = %+v, %v", result, err)
			}
		})
	}
	wg.Wait()
	current, err := store.List(t.Context())
	if err != nil || current[0].Revision != 2 {
		t.Fatalf("current = %+v, %v", current, err)
	}
}

func TestReceiptCapacityPreservesOriginalRecovery(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	input := Update{ID: ExampleReviewID, ExpectedRevision: 1, Status: Resolved}
	accepted, err := store.Update(t.Context(), "original", input)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.db.ExecContext(t.Context(), fmt.Sprintf(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < %d) INSERT INTO invocations SELECT 'fill-' || i, '{"id":"missing","expectedRevision":1,"status":"open"}', '{"type":"notFound"}' FROM n`, MaxReceipts-1))
	if err != nil {
		t.Fatal(err)
	}
	refused, err := store.Update(t.Context(), "new", Update{ID: ExampleReviewID, ExpectedRevision: 2, Status: Open})
	if err != nil || refused.Type != Capacity {
		t.Fatalf("capacity = %+v, %v", refused, err)
	}
	recovered, err := store.Update(t.Context(), "original", input)
	if err != nil || !reflect.DeepEqual(recovered, accepted) {
		t.Fatalf("recovery at capacity = %+v, %v", recovered, err)
	}
}

func TestInvalidInputsCannotConsumeInvocationOrAdvanceReview(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	input := Update{ID: ExampleReviewID, ExpectedRevision: 1, Status: Resolved}
	for _, id := range []string{"", strings.Repeat("x", 257), "invalid\xff"} {
		if _, err := store.Update(t.Context(), id, input); err == nil {
			t.Fatalf("accepted invalid identity %q", id)
		}
	}
	for _, invalid := range []Update{
		{ID: strings.Repeat("x", 129), ExpectedRevision: 1, Status: Resolved},
		{ID: "invalid\xff", ExpectedRevision: 1, Status: Resolved},
		{ID: ExampleReviewID, Status: Resolved},
		{ID: ExampleReviewID, ExpectedRevision: MaxRevision + 1, Status: Resolved},
		{ID: ExampleReviewID, ExpectedRevision: 1, Status: "unknown"},
	} {
		result, err := store.Update(t.Context(), "original", invalid)
		if err != nil || result.Type != Invalid {
			t.Fatalf("invalid input = %+v, %v", result, err)
		}
	}
	result, err := store.Update(t.Context(), "original", input)
	if err != nil || result.Type != Updated || result.Review.Revision != 2 {
		t.Fatalf("admitted input after invalid proposals = %+v, %v", result, err)
	}
}

func TestRevisionExhaustionIsAKnownRefusal(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	if _, err := store.db.ExecContext(t.Context(), `UPDATE reviews SET revision = ?`, MaxRevision); err != nil {
		t.Fatal(err)
	}
	result, err := store.Update(t.Context(), "original", Update{ID: ExampleReviewID, ExpectedRevision: MaxRevision, Status: Resolved})
	if err != nil || result.Type != RevisionExhausted {
		t.Fatalf("exhausted revision = %+v, %v", result, err)
	}
	current, err := store.List(t.Context())
	if err != nil || current[0].Revision != MaxRevision || current[0].Status != Open {
		t.Fatalf("exhausted update changed review: %+v, %v", current, err)
	}
}
