package reviews

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
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
	accepted, err := testUpdate(t.Context(), first, "original-call", input)
	if err != nil || accepted.Type != Updated || accepted.Review.Revision != 2 {
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
	input.Status = Open
	conflicting, err := testUpdate(t.Context(), restarted, "original-call", input)
	if err != nil || conflicting.Type != IdentityConflict {
		t.Fatalf("identity reuse = %+v, %v", conflicting, err)
	}
	input.Status = Resolved
	stale, err := testUpdate(t.Context(), restarted, "another-call", input)
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
	if _, err := testUpdate(t.Context(), store, "original-call", input); err == nil {
		t.Fatal("receipt failure became success")
	}
	invalid := input
	invalid.ExpectedRevision = 0
	if refused, err := testUpdate(t.Context(), store, "refused-call", invalid); err == nil || refused.Type != "" {
		t.Fatalf("failed refusal receipt became a known result: %+v, %v", refused, err)
	}
	current, err := store.List(t.Context())
	if err != nil || current[0].Revision != 1 || current[0].Status != Open {
		t.Fatalf("failed commit advanced review: %+v, %v", current, err)
	}
	if _, err := store.db.ExecContext(t.Context(), `DROP TRIGGER reject_receipt`); err != nil {
		t.Fatal(err)
	}
	result, err := testUpdate(t.Context(), store, "original-call", input)
	if err != nil || result.Type != Updated || result.Review.Revision != 2 {
		t.Fatalf("original call = %+v, %v", result, err)
	}
}

func TestConcurrentDuplicateHasOneBusinessTransition(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			result, err := testUpdate(t.Context(), store, "same-call", Update{ID: ExampleReviewID, ExpectedRevision: 1, Status: Resolved})
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

func TestReceiptHistoryDoesNotExhaustMutationAdmission(t *testing.T) {
	directory := t.TempDir()
	store := openTestStore(t, directory)
	input := Update{ID: ExampleReviewID, ExpectedRevision: 1, Status: Resolved}
	accepted, err := testUpdate(t.Context(), store, "original", input)
	if err != nil || accepted.Type != Updated || accepted.Review == nil {
		t.Fatalf("original update = %+v, %v", accepted, err)
	}
	// Historical evidence cannot consume a permanent mutation quota. Reach the
	// former cutoff through the owner, including actual refused-call receipts.
	for index := range 4095 {
		result, err := testUpdate(t.Context(), store, fmt.Sprintf("history-%d", index), Update{ID: "missing", ExpectedRevision: 1, Status: Open})
		if err != nil || result.Type != NotFound {
			t.Fatalf("historical refusal = %+v, %v", result, err)
		}
	}
	changed := Update{ID: ExampleReviewID, ExpectedRevision: 2, Status: Open}
	advanced, err := testUpdate(t.Context(), store, "after-history", changed)
	if err != nil || advanced.Type != Updated || advanced.Review == nil || advanced.Review.Revision != 3 || advanced.Review.Status != Open {
		t.Fatalf("history exhausted new business work: %+v, %v", advanced, err)
	}
	refused, err := testUpdate(t.Context(), store, "refused-after-history", changed)
	if err != nil || refused.Type != Conflict || refused.Review == nil || *refused.Review != *advanced.Review {
		t.Fatalf("business refusal lost evidence: %+v, %v", refused, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openTestStore(t, directory)
	contracts, err := NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	for _, original := range []struct {
		id    string
		input Update
		want  UpdateResult
	}{{"original", input, accepted}, {"after-history", changed, advanced}, {"refused-after-history", changed, refused}} {
		admitted, err := jsoninput.Encode(original.input)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := store.Receipt(t.Context(), pluginsdk.ReceiptRequest{ID: original.id, Method: contracts.Update.Name(), InputDigest: admitted.Digest()}, contracts.Update.Name())
		if err != nil || !receipt.Found {
			t.Fatalf("missing original receipt %s: %+v, %v", original.id, receipt, err)
		}
		recovered, err := pluginsdk.DecodeReply[UpdateResult](receipt.Reply)
		if err != nil || !reflect.DeepEqual(recovered, original.want) {
			t.Fatalf("changed original receipt %s: %+v, %v", original.id, recovered, err)
		}
	}
	current, err := store.List(t.Context())
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
	input := Update{ID: ExampleReviewID, ExpectedRevision: 1, Status: Resolved}
	for _, id := range []string{"", strings.Repeat("x", 257), "invalid\xff"} {
		if _, err := testUpdate(t.Context(), store, id, input); err == nil {
			t.Fatalf("accepted invalid identity %q", id)
		}
	}
	for _, invalid := range []Update{
		{ID: strings.Repeat("x", 129), ExpectedRevision: 1, Status: Resolved},
		{ID: ExampleReviewID, ExpectedRevision: 1, Status: "unknown"},
	} {
		result, err := testUpdate(t.Context(), store, "original", invalid)
		if err == nil || result.Type != "" {
			t.Fatalf("malformed input became an admitted result: %+v, %v", result, err)
		}
	}
	if _, err := testUpdate(t.Context(), store, "original", Update{ID: "invalid\xff", ExpectedRevision: 1, Status: Resolved}); err == nil {
		t.Fatal("invalid UTF-8 acquired admitted JSON or consumed an invocation")
	}
	result, err := testUpdate(t.Context(), store, "original", input)
	if err != nil || result.Type != Updated || result.Review.Revision != 2 {
		t.Fatalf("admitted input after invalid proposals = %+v, %v", result, err)
	}
}

func TestInvalidBusinessInputRetainsItsOriginalReceiptAcrossRestart(t *testing.T) {
	for _, invalid := range []Update{
		{ID: "", ExpectedRevision: 1, Status: Resolved},
		{ID: ExampleReviewID, Status: Resolved},
		{ID: ExampleReviewID, ExpectedRevision: -1, Status: Resolved},
		{ID: ExampleReviewID, ExpectedRevision: exactint.Maximum + 1, Status: Resolved},
		{ID: strings.Repeat("界", 43), ExpectedRevision: 1, Status: Resolved},
	} {
		t.Run(fmt.Sprintf("%s/%d", invalid.ID, invalid.ExpectedRevision), func(t *testing.T) {
			directory := t.TempDir()
			store := openTestStore(t, directory)
			for range 2 {
				refused, err := testUpdate(t.Context(), store, "refused", invalid)
				if err != nil || refused.Type != Invalid || refused.Review != nil {
					t.Fatalf("business refusal = %+v, %v", refused, err)
				}
			}
			valid := Update{ID: ExampleReviewID, ExpectedRevision: 1, Status: Resolved}
			if changed, err := testUpdate(t.Context(), store, "refused", valid); err != nil || changed.Type != IdentityConflict {
				t.Fatalf("refused identity accepted changed input: %+v, %v", changed, err)
			}
			if accepted, err := testUpdate(t.Context(), store, "next", valid); err != nil || accepted.Type != Updated || accepted.Review == nil || accepted.Review.Revision != 2 {
				t.Fatalf("fresh identity lost independent business work: %+v, %v", accepted, err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store = openTestStore(t, directory)
			contracts, err := NewContracts()
			if err != nil {
				t.Fatal(err)
			}
			admitted, err := jsoninput.Encode(invalid)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := store.Receipt(t.Context(), pluginsdk.ReceiptRequest{ID: "refused", Method: contracts.Update.Name(), InputDigest: admitted.Digest()}, contracts.Update.Name())
			if err != nil || !receipt.Found {
				t.Fatalf("lost original invalid-input receipt: %+v, %v", receipt, err)
			}
			result, err := pluginsdk.DecodeReply[UpdateResult](receipt.Reply)
			if err != nil || result.Type != Invalid || result.Review != nil {
				t.Fatalf("invalid-input receipt changed: %+v, %v", result, err)
			}
			var receipts int
			if err := store.db.QueryRowContext(t.Context(), `SELECT count(*) FROM invocations`).Scan(&receipts); err != nil || receipts != 2 {
				t.Fatalf("refusal or success was lost or duplicated: %d, %v", receipts, err)
			}
			current, err := store.List(t.Context())
			if err != nil || len(current) != 1 || current[0].Revision != 2 || current[0].Status != Resolved {
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
	result, err := testUpdate(t.Context(), store, "original", Update{ID: ExampleReviewID, ExpectedRevision: exactint.Maximum, Status: Resolved})
	if err != nil || result.Type != RevisionExhausted {
		t.Fatalf("exhausted revision = %+v, %v", result, err)
	}
	current, err := store.List(t.Context())
	if err != nil || current[0].Revision != exactint.Maximum || current[0].Status != Open {
		t.Fatalf("exhausted update changed review: %+v, %v", current, err)
	}
}

func testUpdate(ctx context.Context, store *Store, id string, input Update) (UpdateResult, error) {
	admitted, err := jsoninput.Encode(input)
	if err != nil {
		return UpdateResult{}, err
	}
	return store.Update(ctx, id, input, admitted)
}
