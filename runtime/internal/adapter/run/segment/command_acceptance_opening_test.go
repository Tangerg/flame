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
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func acceptedRootOpening(t *testing.T, commitRaw string, draft run.Draft, key, fingerprint, userItemID string) runs.OpeningCommit {
	t.Helper()
	opening := mustAdmissionOpening(t, testCommitID(commitRaw), draft, nil, nil, nil, nil)
	acceptance, err := runs.AcceptCommand(key, fingerprint, opening, userItemID)
	if err != nil {
		t.Fatalf("AcceptCommand: %v", err)
	}
	opening, err = opening.WithCommandAcceptance(acceptance)
	if err != nil {
		t.Fatalf("WithCommandAcceptance: %v", err)
	}
	return opening
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
	if err := effects.CommitOpening(ctx, acceptedRootOpening(t, "run_commit_accept", first, "start-key", fingerprint, "item_accept")); err != nil {
		t.Fatalf("CommitOpening: %v", err)
	}
	if _, found, runErr := runStore.Run(ctx, first.RunID); runErr != nil || !found {
		t.Fatalf("admitted run: found=%v error=%v", found, runErr)
	}
	record, err := acceptances.Lookup(ctx, "start-key")
	if err != nil || !record.Found || !record.MaterialAvailable || record.Acceptance.RunID != first.RunID {
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
