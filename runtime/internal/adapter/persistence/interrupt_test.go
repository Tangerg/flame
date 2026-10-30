package persistence

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// seededInterruptStore opens each hand-off after writing the Runs it names.
type seededInterruptStore struct {
	*InterruptStore
	db *sql.DB
	t  *testing.T
}

func (s seededInterruptStore) Open(ctx context.Context, pending runs.Pending) error {
	seedParkedRuns(s.t, s.db, "ses_"+pending.RootRunID, pending)
	return s.InterruptStore.Open(ctx, pending)
}

func interruptStoreFixture(t *testing.T) seededInterruptStore {
	t.Helper()
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return seededInterruptStore{InterruptStore: newInterruptStore(db), db: db, t: t}
}

func TestInterruptStore_OpenGetListDelete(t *testing.T) {
	ctx := context.Background()
	store := interruptStoreFixture(t)

	p := runs.Pending{
		RootRunID:  "run_1",
		ExecutorID: "turn_1",
		Interrupts: runs.OpenInterruptsOf([]transcript.Interrupt{{
			ItemID: "item_question", ItemOccurredAt: time.Unix(2, 0).UTC(), RunID: "run_1", Kind: run.InterruptQuestion,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Choose", Kind: questioncontract.Text}}},
		}}),
		Bindings: []runs.InterruptBinding{{
			InterruptItemID: "item_question",
			MemberID:        "member_1",
			RequestID:       "request_1",
		}},
		Continuations: []runs.Continuation{{
			RunID:    "run_1",
			MemberID: "member_1",
		}},
		CreatedAt: time.Unix(5, 0).UTC()}
	if err := store.Open(ctx, p); err != nil {
		t.Fatalf("Open: %v", err)
	}
	// A second barrier cannot overwrite the one already waiting for a decision.
	if err := store.Open(ctx, p); !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("Open duplicate error = %v, want ErrIdentityConflict", err)
	}

	got, ok, err := store.Get(ctx, "run_1")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if len(got.Interrupts) != 1 ||
		got.Interrupts[0].ItemID != "item_question" || got.Interrupts[0].Approval != nil ||
		!got.CreatedAt.Equal(time.Unix(5, 0).UTC()) {
		t.Fatalf("Get returned %+v", got)
	}

	if list, _ := store.List(ctx, "ses_run_1"); len(list) != 1 {
		t.Fatalf("List(root Run Session) = %d, want 1", len(list))
	}
	if list, _ := store.List(ctx, ""); len(list) != 1 {
		t.Fatalf("List(all) = %d, want 1", len(list))
	}
	if list, _ := store.List(ctx, "nope"); len(list) != 0 {
		t.Fatalf("List(nope) = %d, want 0", len(list))
	}

	if err := store.Delete(ctx, "run_1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ctx, "run_1"); err != nil {
		t.Fatalf("Delete not idempotent: %v", err)
	}
	if _, ok, _ := store.Get(ctx, "run_1"); ok {
		t.Fatal("Get after Delete: still present")
	}
}

// TestInterruptStore_ConsumeIsAtomic pins the resume-idempotency fix: Consume
// reads AND deletes the pending interrupt in one statement, so two concurrent
// resumes can't both claim it (the second gets ok=false and backs off, instead
// of both rebuilding the parked executor tree and re-firing the approved tool). Also
// exercises that modernc SQLite supports DELETE ... RETURNING.
func TestInterruptStore_ConsumeIsAtomic(t *testing.T) {
	ctx := context.Background()
	store := interruptStoreFixture(t)
	arguments, err := tool.ParseArguments(`{"command":"go test ./..."}`)
	if err != nil {
		t.Fatalf("parse approval arguments: %v", err)
	}

	// Nothing recorded → ok=false.
	if _, ok, consumeErr := store.Consume(ctx, "run_x"); consumeErr != nil || ok {
		t.Fatalf("Consume(empty) = ok=%v err=%v, want ok=false", ok, consumeErr)
	}

	if openErr := store.Open(ctx, runs.Pending{
		RootRunID:  "run_1",
		ExecutorID: "turn_1",
		Interrupts: runs.OpenInterruptsOf([]transcript.Interrupt{{
			ItemID: "item_approval", ItemOccurredAt: time.Unix(2, 0).UTC(), RunID: "run_1", Kind: run.InterruptApproval,
			Approval: &transcript.Approval{
				Tool: transcript.ToolInvocation{Name: "shell", Arguments: arguments},
				Risk: tool.RiskHigh, Reason: "executes tests", Rememberable: true,
			},
		}}),
		Bindings: []runs.InterruptBinding{{
			InterruptItemID: "item_approval",
			MemberID:        "member_1",
			RequestID:       "request_1",
			ToolCallID:      "call_1",
		}},
		Continuations: []runs.Continuation{{
			RunID: "run_1", MemberID: "member_1",
		}},
		CreatedAt: time.Unix(7, 0).UTC()}); openErr != nil {
		t.Fatalf("Open: %v", openErr)
	}

	// First consume returns the full record.
	got, ok, err := store.Consume(ctx, "run_1")
	if err != nil || !ok {
		t.Fatalf("Consume: ok=%v err=%v", ok, err)
	}
	root, _ := got.RootContinuation()
	if root.MemberID != "member_1" || len(got.Interrupts) != 1 || got.Interrupts[0].ItemID != "item_approval" ||
		got.Bindings[0].ToolCallID != "call_1" ||
		got.Interrupts[0].Approval == nil || got.Interrupts[0].Approval.Risk != tool.RiskHigh ||
		got.Interrupts[0].Approval.Reason != "executes tests" || !got.Interrupts[0].Approval.Rememberable {
		t.Fatalf("Consume returned %+v", got)
	}

	// Second consume finds nothing — the record was removed atomically with
	// the read, so a racing resume can't re-fire the tool.
	if _, ok, err := store.Consume(ctx, "run_1"); err != nil || ok {
		t.Fatalf("second Consume = ok=%v err=%v, want ok=false — record must be gone", ok, err)
	}
}

func TestInterruptStoreRoundTripsContinuationsWithoutExecutorTopology(t *testing.T) {
	store := interruptStoreFixture(t)
	createdAt := time.Unix(10, 0).UTC()
	pending := runs.Pending{
		RootRunID:  "run_root",
		ExecutorID: "turn_1",
		Interrupts: runs.OpenInterruptsOf([]transcript.Interrupt{{
			ItemID: "item_child", ItemOccurredAt: time.Unix(2, 0).UTC(), RunID: "run_child", Kind: run.InterruptQuestion,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: questioncontract.Text}}},
		}}),
		Bindings: []runs.InterruptBinding{{
			InterruptItemID: "item_child",
			MemberID:        "member_child",
			RequestID:       "request_child",
		}},
		Continuations: []runs.Continuation{
			{
				RunID:    "run_child",
				MemberID: "member_child",
				DrainedTools: []runs.DrainedTool{{
					ItemID: "item_open",
					CallID: "call_open", SourceCallID: "provider_open",
				}},
			},
			{
				RunID:    "run_root",
				MemberID: "member_root",
				DrainedTools: []runs.DrainedTool{{
					ItemID: "item_spawn_child", CallID: "call_child", SourceCallID: "provider_child",
				}},
			},
		},
		CreatedAt: createdAt.Add(time.Second)}
	if err := store.Open(t.Context(), pending); err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, found, err := store.Get(t.Context(), pending.RootRunID)
	if err != nil || !found {
		t.Fatalf("Get: found=%v err=%v", found, err)
	}
	child := got.Continuations[0]
	if child.RunID != "run_child" || child.MemberID != "member_child" || len(child.DrainedTools) != 1 ||
		child.DrainedTools[0].CallID != "call_open" ||
		child.DrainedTools[0].SourceCallID != "provider_open" {
		t.Fatalf("child continuation = %+v", child)
	}
	root, found := got.RootContinuation()
	if !found ||
		len(root.DrainedTools) != 1 ||
		root.DrainedTools[0].ItemID != "item_spawn_child" ||
		root.DrainedTools[0].CallID != "call_child" ||
		root.DrainedTools[0].SourceCallID != "provider_child" {
		t.Fatalf("root drained tools = %+v, want unfinished child call hand-off", root.DrainedTools)
	}
}

func TestInterruptStoreRejectsUnknownExecutorTopologyFields(t *testing.T) {
	database, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store := newInterruptStore(database)
	pending := runs.Pending{
		RootRunID: "run_root", ExecutorID: "turn_1",
		Interrupts: runs.OpenInterruptsOf([]transcript.Interrupt{{
			ItemID: "item_question", ItemOccurredAt: time.Unix(2, 0).UTC(), RunID: "run_root", Kind: run.InterruptQuestion,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: questioncontract.Text}}},
		}}),
		Bindings: []runs.InterruptBinding{{
			InterruptItemID: "item_question", MemberID: "member_root", RequestID: "request_root",
		}},
		Continuations: []runs.Continuation{{
			RunID: "run_root", MemberID: "member_root",
		}},
		CreatedAt: time.Unix(2, 0).UTC()}
	seedParkedRuns(t, database, "session_1", pending)
	if err := store.Open(t.Context(), pending); err != nil {
		t.Fatalf("Open interrupt: %v", err)
	}
	var encoded string
	if err := database.QueryRowContext(
		t.Context(),
		`SELECT continuations FROM interrupts WHERE root_run_id = ?`,
		pending.RootRunID,
	).Scan(&encoded); err != nil {
		t.Fatalf("read continuation JSON: %v", err)
	}
	poisoned := strings.Replace(
		encoded,
		`"memberId":"member_root"`,
		`"memberId":"member_root","parentProcessId":"legacy_parent"`,
		1,
	)
	if poisoned == encoded {
		t.Fatalf("continuation JSON does not contain member identity: %s", encoded)
	}
	if _, err := database.ExecContext(
		t.Context(),
		`UPDATE interrupts SET continuations = ? WHERE root_run_id = ?`,
		poisoned,
		pending.RootRunID,
	); err != nil {
		t.Fatalf("inject unknown topology field: %v", err)
	}
	if _, _, err := store.Get(t.Context(), pending.RootRunID); !errors.Is(err, json.ErrUnknownName) || !strings.Contains(err.Error(), `"parentProcessId"`) {
		t.Fatalf("Get error = %v, want unknown executor topology field", err)
	}
}

func TestInterruptStoreExecutorRootHasOnePendingOwner(t *testing.T) {
	store := interruptStoreFixture(t)
	ctx := t.Context()
	for _, runID := range []string{"run_1", "run_2"} {
		err := store.Open(ctx, runs.Pending{
			RootRunID: runID, ExecutorID: "turn_" + runID,
			Interrupts: runs.OpenInterruptsOf([]transcript.Interrupt{{
				ItemID: "item_" + runID, ItemOccurredAt: time.Unix(2, 0).UTC(), RunID: runID,
				Kind: run.InterruptQuestion, Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "continue?", Kind: questioncontract.Text}}},
			}}),
			Bindings: []runs.InterruptBinding{{
				InterruptItemID: "item_" + runID,
				MemberID:        "member_shared",
				RequestID:       "request_" + runID,
			}},
			Continuations: []runs.Continuation{{
				RunID: runID, MemberID: "member_shared",
			}},
			CreatedAt: time.Unix(2, 0).UTC()})
		if runID == "run_1" && err != nil {
			t.Fatalf("first Open: %v", err)
		}
		if runID == "run_2" && err == nil {
			t.Fatal("second Open reused an executor checkpoint root owned by another Pending")
		}
	}
}
