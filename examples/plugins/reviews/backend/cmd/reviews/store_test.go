package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend/contract"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
)

func openTestStore(t *testing.T, directory string) *store {
	t.Helper()
	store, err := openStore(t.Context(), directory)
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
	first, err := openStore(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	input := reviews.Update{ID: reviews.ExampleReviewID, ExpectedRevision: 1, Status: reviews.Resolved}
	accepted, err := testUpdate(t.Context(), first, "original-call", input)
	if err != nil || accepted.Type != reviews.Updated || accepted.Review.Revision != 2 {
		t.Fatalf("update = %+v, %v", accepted, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	restarted := openTestStore(t, directory)
	replayed, err := testUpdate(t.Context(), restarted, "original-call", input)
	if err != nil || !reflect.DeepEqual(replayed, accepted) {
		t.Fatalf("replay = %+v, %v, want %+v", replayed, err, accepted)
	}
	input.Status = reviews.Open
	conflicting, err := testUpdate(t.Context(), restarted, "original-call", input)
	if err != nil || conflicting.Type != reviews.IdentityConflict {
		t.Fatalf("identity reuse = %+v, %v", conflicting, err)
	}
	input.Status = reviews.Resolved
	stale, err := testUpdate(t.Context(), restarted, "another-call", input)
	if err != nil || stale.Type != reviews.Conflict || stale.Review.Revision != 2 {
		t.Fatalf("stale update = %+v, %v", stale, err)
	}
	current, err := restarted.list(t.Context())
	if err != nil || len(current) != 1 || !reflect.DeepEqual(current[0], *accepted.Review) {
		t.Fatalf("current = %+v, %v", current, err)
	}
}

func TestReceiptFailureRollsBackBusinessTransition(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	if _, err := store.db.ExecContext(t.Context(), `CREATE TRIGGER reject_receipt BEFORE INSERT ON invocations BEGIN SELECT RAISE(ABORT, 'receipt rejected'); END`); err != nil {
		t.Fatal(err)
	}
	input := reviews.Update{ID: reviews.ExampleReviewID, ExpectedRevision: 1, Status: reviews.Resolved}
	if _, err := testUpdate(t.Context(), store, "original-call", input); err == nil {
		t.Fatal("receipt failure became success")
	}
	invalid := input
	invalid.ExpectedRevision = 0
	if refused, err := testUpdate(t.Context(), store, "refused-call", invalid); err == nil || refused.Type != "" {
		t.Fatalf("failed refusal receipt became a known result: %+v, %v", refused, err)
	}
	current, err := store.list(t.Context())
	if err != nil || current[0].Revision != 1 || current[0].Status != reviews.Open {
		t.Fatalf("failed commit advanced review: %+v, %v", current, err)
	}
	if _, err := store.db.ExecContext(t.Context(), `DROP TRIGGER reject_receipt`); err != nil {
		t.Fatal(err)
	}
	result, err := testUpdate(t.Context(), store, "original-call", input)
	if err != nil || result.Type != reviews.Updated || result.Review.Revision != 2 {
		t.Fatalf("original call = %+v, %v", result, err)
	}
}

func TestConcurrentDuplicateHasOneBusinessTransition(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			result, err := testUpdate(t.Context(), store, "same-call", reviews.Update{ID: reviews.ExampleReviewID, ExpectedRevision: 1, Status: reviews.Resolved})
			if err != nil || result.Type != reviews.Updated || result.Review.Revision != 2 {
				t.Errorf("duplicate = %+v, %v", result, err)
			}
		})
	}
	wg.Wait()
	current, err := store.list(t.Context())
	if err != nil || current[0].Revision != 2 {
		t.Fatalf("current = %+v, %v", current, err)
	}
}

func TestReceiptHistoryDoesNotExhaustMutationAdmission(t *testing.T) {
	directory := t.TempDir()
	store := openTestStore(t, directory)
	input := reviews.Update{ID: reviews.ExampleReviewID, ExpectedRevision: 1, Status: reviews.Resolved}
	accepted, err := testUpdate(t.Context(), store, "original", input)
	if err != nil || accepted.Type != reviews.Updated || accepted.Review == nil {
		t.Fatalf("original update = %+v, %v", accepted, err)
	}
	// Historical evidence cannot consume a permanent mutation quota. Reach the
	// former cutoff through the owner, including actual refused-call receipts.
	for index := range 4095 {
		result, err := testUpdate(t.Context(), store, fmt.Sprintf("history-%d", index), reviews.Update{ID: "missing", ExpectedRevision: 1, Status: reviews.Open})
		if err != nil || result.Type != reviews.NotFound {
			t.Fatalf("historical refusal = %+v, %v", result, err)
		}
	}
	changed := reviews.Update{ID: reviews.ExampleReviewID, ExpectedRevision: 2, Status: reviews.Open}
	advanced, err := testUpdate(t.Context(), store, "after-history", changed)
	if err != nil || advanced.Type != reviews.Updated || advanced.Review == nil || advanced.Review.Revision != 3 || advanced.Review.Status != reviews.Open {
		t.Fatalf("history exhausted new business work: %+v, %v", advanced, err)
	}
	refused, err := testUpdate(t.Context(), store, "refused-after-history", changed)
	if err != nil || refused.Type != reviews.Conflict || refused.Review == nil || *refused.Review != *advanced.Review {
		t.Fatalf("business refusal lost evidence: %+v, %v", refused, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openTestStore(t, directory)
	contracts, err := reviews.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	for _, original := range []struct {
		id    string
		input reviews.Update
		want  reviews.UpdateResult
	}{{"original", input, accepted}, {"after-history", changed, advanced}, {"refused-after-history", changed, refused}} {
		admitted, err := jsonvalue.Encode(original.input)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := store.receipt(t.Context(), qimumcp.ReceiptRequest{ID: original.id, Method: contracts.Update.Name(), InputDigest: admitted.Digest()}, contracts.Update.Name())
		if err != nil || !receipt.Found {
			t.Fatalf("missing original receipt %s: %+v, %v", original.id, receipt, err)
		}
		recovered, err := qimumcp.DecodeReply[reviews.UpdateResult](receipt.Reply)
		if err != nil || !reflect.DeepEqual(recovered, original.want) {
			t.Fatalf("changed original receipt %s: %+v, %v", original.id, recovered, err)
		}
	}
	current, err := store.list(t.Context())
	if err != nil || len(current) != 1 || current[0] != *advanced.Review {
		t.Fatalf("receipt reads changed the review: %+v, %v", current, err)
	}
	var receipts int
	if err := store.db.QueryRowContext(t.Context(), `SELECT count(*) FROM invocations`).Scan(&receipts); err != nil || receipts != 4098 {
		t.Fatalf("history was lost or recovery wrote receipts: %d, %v", receipts, err)
	}
}

func TestMalformedInputsCannotConsumeInvocationOrAdvanceReview(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	input := reviews.Update{ID: reviews.ExampleReviewID, ExpectedRevision: 1, Status: reviews.Resolved}
	for _, id := range []string{"", strings.Repeat("x", 257), "invalid\xff"} {
		if _, err := testUpdate(t.Context(), store, id, input); err == nil {
			t.Fatalf("accepted invalid identity %q", id)
		}
	}
	for _, invalid := range []reviews.Update{
		{ID: strings.Repeat("x", 129), ExpectedRevision: 1, Status: reviews.Resolved},
		{ID: reviews.ExampleReviewID, ExpectedRevision: 1, Status: "unknown"},
	} {
		result, err := testUpdate(t.Context(), store, "original", invalid)
		if err == nil || result.Type != "" {
			t.Fatalf("malformed input became an admitted result: %+v, %v", result, err)
		}
	}
	if _, err := testUpdate(t.Context(), store, "original", reviews.Update{ID: "invalid\xff", ExpectedRevision: 1, Status: reviews.Resolved}); err == nil {
		t.Fatal("invalid UTF-8 acquired admitted JSON or consumed an invocation")
	}
	result, err := testUpdate(t.Context(), store, "original", input)
	if err != nil || result.Type != reviews.Updated || result.Review.Revision != 2 {
		t.Fatalf("admitted input after invalid proposals = %+v, %v", result, err)
	}
}

func TestInvalidBusinessInputRetainsItsOriginalReceiptAcrossRestart(t *testing.T) {
	for _, invalid := range []reviews.Update{
		{ID: "", ExpectedRevision: 1, Status: reviews.Resolved},
		{ID: reviews.ExampleReviewID, Status: reviews.Resolved},
		{ID: reviews.ExampleReviewID, ExpectedRevision: -1, Status: reviews.Resolved},
		{ID: reviews.ExampleReviewID, ExpectedRevision: exactint.Maximum + 1, Status: reviews.Resolved},
		{ID: strings.Repeat("界", 43), ExpectedRevision: 1, Status: reviews.Resolved},
	} {
		t.Run(fmt.Sprintf("%s/%d", invalid.ID, invalid.ExpectedRevision), func(t *testing.T) {
			directory := t.TempDir()
			store := openTestStore(t, directory)
			for range 2 {
				refused, err := testUpdate(t.Context(), store, "refused", invalid)
				if err != nil || refused.Type != reviews.Invalid || refused.Review != nil {
					t.Fatalf("business refusal = %+v, %v", refused, err)
				}
			}
			valid := reviews.Update{ID: reviews.ExampleReviewID, ExpectedRevision: 1, Status: reviews.Resolved}
			if changed, err := testUpdate(t.Context(), store, "refused", valid); err != nil || changed.Type != reviews.IdentityConflict {
				t.Fatalf("refused identity accepted changed input: %+v, %v", changed, err)
			}
			if accepted, err := testUpdate(t.Context(), store, "next", valid); err != nil || accepted.Type != reviews.Updated || accepted.Review == nil || accepted.Review.Revision != 2 {
				t.Fatalf("fresh identity lost independent business work: %+v, %v", accepted, err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store = openTestStore(t, directory)
			contracts, err := reviews.NewContracts()
			if err != nil {
				t.Fatal(err)
			}
			admitted, err := jsonvalue.Encode(invalid)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := store.receipt(t.Context(), qimumcp.ReceiptRequest{ID: "refused", Method: contracts.Update.Name(), InputDigest: admitted.Digest()}, contracts.Update.Name())
			if err != nil || !receipt.Found {
				t.Fatalf("lost original invalid-input receipt: %+v, %v", receipt, err)
			}
			result, err := qimumcp.DecodeReply[reviews.UpdateResult](receipt.Reply)
			if err != nil || result.Type != reviews.Invalid || result.Review != nil {
				t.Fatalf("invalid-input receipt changed: %+v, %v", result, err)
			}
			var receipts int
			if err := store.db.QueryRowContext(t.Context(), `SELECT count(*) FROM invocations`).Scan(&receipts); err != nil || receipts != 2 {
				t.Fatalf("refusal or success was lost or duplicated: %d, %v", receipts, err)
			}
			current, err := store.list(t.Context())
			if err != nil || len(current) != 1 || current[0].Revision != 2 || current[0].Status != reviews.Resolved {
				t.Fatalf("refusal/recovery advanced the review: %+v, %v", current, err)
			}
		})
	}
}

func TestRevisionExhaustionIsAKnownRefusal(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	if _, err := store.db.ExecContext(t.Context(), `UPDATE reviews SET revision = ?`, int64(exactint.Maximum)); err != nil {
		t.Fatal(err)
	}
	result, err := testUpdate(t.Context(), store, "original", reviews.Update{ID: reviews.ExampleReviewID, ExpectedRevision: exactint.Maximum, Status: reviews.Resolved})
	if err != nil || result.Type != reviews.RevisionExhausted {
		t.Fatalf("exhausted revision = %+v, %v", result, err)
	}
	current, err := store.list(t.Context())
	if err != nil || current[0].Revision != exactint.Maximum || current[0].Status != reviews.Open {
		t.Fatalf("exhausted update changed review: %+v, %v", current, err)
	}
}

func testUpdate(ctx context.Context, store *store, id string, input reviews.Update) (reviews.UpdateResult, error) {
	admitted, err := jsonvalue.Encode(input)
	if err != nil {
		return reviews.UpdateResult{}, err
	}
	return store.update(ctx, id, input, admitted)
}
