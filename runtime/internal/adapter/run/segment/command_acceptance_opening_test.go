package segment

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func acceptedRootOpening(t *testing.T, commitRaw string, draft run.Draft, key, fingerprint, userItemID string) runs.OpeningCommit {
	t.Helper()
	item, err := transcript.NewUserMessage(transcript.ItemIdentity{SessionID: draft.SessionID, RunID: draft.RunID, ItemID: userItemID, OccurredAt: draft.CreatedAt}, []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "start"}})
	if err != nil {
		t.Fatal(err)
	}
	opening := mustAdmissionOpening(t, testCommitID(commitRaw), draft, nil, nil, nil, []runs.EventCommit{{SessionID: draft.SessionID, RunID: draft.RunID, SegmentID: draft.SegmentID, Items: []transcript.Item{item}}})
	acceptance, err := runs.AcceptCommand(key, fingerprint, opening)
	if err != nil {
		t.Fatalf("AcceptCommand: %v", err)
	}
	opening, err = opening.WithCommandAcceptance(acceptance)
	if err != nil {
		t.Fatalf("WithCommandAcceptance: %v", err)
	}
	return opening
}

func TestResponseOnlyResumeAcceptanceCommitsWithAnswersAndRejectsKeyReuse(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state, items := sqlite.NewRunStore(db), sqlite.NewTranscriptStore(db)
	interrupts := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
	commands := persistence.NewCommandAcceptanceStore(db)
	bindings := persistence.NewAcceptedContinuationStore(sqlite.NewAcceptedContinuationStore(db))
	effects := sqliteEffects(sqliteOpeningStores{interrupts: interrupts, transcript: items}, Config{
		State: state, CommandAcceptances: commands, AcceptedContinuations: bindings,
		Tx: func(ctx context.Context, apply func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, apply)
		},
	})
	for index := range 2 {
		suffix := []string{"first", "second"}[index]
		sessionID, runID := "ses_"+suffix, "run_"+suffix
		pending := singleRunPending(t, runID, sessionID, "member_"+suffix, "request_"+suffix, "item_"+suffix, time.Unix(3, 0).UTC())
		seedPending(t, db, sessionID, pending, questionCapabilities())
		if err := interrupts.Open(ctx, pending); err != nil {
			t.Fatal(err)
		}
		if err := items.AppendItem(ctx, singleRunQuestionItem(t, sessionID, pending)); err != nil {
			t.Fatal(err)
		}
		parked, _, err := state.Run(ctx, runID)
		if err != nil {
			t.Fatal(err)
		}
		answers := mustResumeAcceptance(t, items, pending, []runs.InterruptAnswer{{InterruptItemID: "item_" + suffix, MemberID: "member_" + suffix, RequestID: "request_" + suffix, Resolution: interrupt.Resolution{Approved: true, Answers: [][]string{{"continue"}}}}})
		resume := testsupport.MustResumeRuns(time.Unix(10, 0).UTC(), []run.Run{parked}, []string{"seg_" + suffix})
		opening := mustResumeOpening(t, testCommitID("run_commit_"+suffix), resume, nil, answers)
		acceptance, err := runs.AcceptCommand("resume-once", strings.Repeat("a", 64), opening)
		if err != nil {
			t.Fatal(err)
		}
		opening, err = opening.WithCommandAcceptance(acceptance)
		if err != nil {
			t.Fatal(err)
		}
		err = effects.CommitOpening(ctx, opening)
		if index == 0 {
			if err != nil {
				t.Fatal(err)
			}
			record, err := commands.Lookup(ctx, "resume-once")
			if err != nil || !record.Acceptance.Equal(acceptance) || record.CurrentRun == nil || record.CurrentRun.State() != run.Running {
				t.Fatalf("response-only acceptance=%+v,%v", record, err)
			}
			continue
		}
		if err == nil {
			t.Fatal("duplicate command key committed another continuation")
		}
		current, _, err := state.Run(ctx, runID)
		if err != nil || current.State() != run.Waiting {
			t.Fatalf("key conflict resumed the second Run: %+v,%v", current, err)
		}
		item, _, err := items.Item(ctx, "item_"+suffix)
		question, found := item.Question()
		if err != nil || !found || question.Answers != nil {
			t.Fatalf("key conflict accepted the second answer: %+v,%v", item, err)
		}
	}
}

// TestCommandAcceptanceCommitsAndRollsBackWithTheOpening proves the acceptance and
// the Run it names share one transaction: a fresh opening commits both, and a
// second opening that reuses the key is refused by the global command key, rolling
// the whole opening back so no Run is admitted under a duplicate identity.
func TestCommandAcceptanceCommitsAndRollsBackWithTheOpening(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	runStore := sqlite.NewRunStore(db)
	acceptances := persistence.NewCommandAcceptanceStore(db)
	effects := mustNewEffects(Config{
		State:              runStore,
		Transcript:         sqlite.NewTranscriptStore(db),
		CommandAcceptances: acceptances,
		Tx: func(ctx context.Context, apply func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, apply)
		},
	})
	fingerprint := strings.Repeat("a", 64)

	first := run.Draft{
		RunID: "run_accept", SessionID: "session_accept", SegmentID: "segment_accept",
		ModelSelection: testsupport.DefaultModelSelection(), CreatedAt: time.Unix(1, 0).UTC(),
	}
	opening := acceptedRootOpening(t, "run_commit_accept", first, "start-key", fingerprint, "item_accept")
	wrongInput, _ := opening.CommandAcceptance()
	wrongInput.UserItemID = "item_other"
	if _, err := opening.WithCommandAcceptance(wrongInput); err == nil {
		t.Fatal("command receipt could name an input outside its opening")
	}
	if err := effects.CommitOpening(ctx, opening); err != nil {
		t.Fatalf("CommitOpening: %v", err)
	}
	if _, found, runErr := runStore.Run(ctx, first.RunID); runErr != nil || !found {
		t.Fatalf("admitted run: found=%v error=%v", found, runErr)
	}
	record, err := acceptances.Lookup(ctx, "start-key")
	if err != nil || !record.Found || record.CurrentRun == nil || record.Acceptance.RunID != first.RunID {
		t.Fatalf("committed acceptance = %+v, %v", record, err)
	}

	// A second opening in another Session reusing the key hits the global command
	// key and must roll the whole opening back.
	second := run.Draft{
		RunID: "run_dup", SessionID: "session_other", SegmentID: "segment_dup",
		ModelSelection: testsupport.DefaultModelSelection(), CreatedAt: time.Unix(2, 0).UTC(),
	}
	if err := effects.CommitOpening(ctx, acceptedRootOpening(t, "run_commit_dup", second, "start-key", fingerprint, "item_dup")); err == nil {
		t.Fatal("a reused command key admitted a second Run")
	}
	if _, found, runErr := runStore.Run(ctx, second.RunID); runErr != nil || found {
		t.Fatalf("rolled-back opening left an orphan Run: found=%v error=%v", found, runErr)
	}
	if again, err := acceptances.Lookup(ctx, "start-key"); err != nil || again.Acceptance.RunID != first.RunID {
		t.Fatalf("the refused opening changed the committed acceptance: %+v, %v", again, err)
	}
}
