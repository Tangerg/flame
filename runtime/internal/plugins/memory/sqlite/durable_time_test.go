package sqlite_test

import (
	"math"
	"testing"
	"time"

	agentmemory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	"github.com/Tangerg/flame/runtime/internal/plugins/memory/sqlite"
)

func TestMemoryOwnerTimesRoundTripAtExactBounds(t *testing.T) {
	for _, at := range []time.Time{time.Unix(0, math.MinInt64).UTC(), time.Unix(0, 0).UTC(), time.Unix(0, math.MaxInt64).UTC()} {
		t.Run(at.Format(time.RFC3339Nano), func(t *testing.T) {
			db, err := sqlite.Open(t.Context(), ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			memory := sqlite.NewAgentMemoryStore(db)
			item, added, err := memory.Add(t.Context(), agentmemory.ScopeUser, "", "user content", at)
			if err != nil || !added {
				t.Fatalf("Add = %v, %v", added, err)
			}
			storedItem, found, err := memory.Get(t.Context(), item.ID())
			if err != nil || !found || !storedItem.CreatedAt().Equal(at) || !storedItem.UpdatedAt().Equal(at) {
				t.Fatalf("Memory time = %+v, %v", storedItem.Snapshot(), err)
			}
			facts, err := memory.AppendLedger(t.Context(), agentmemory.FactBatch{
				Project: "/repo", SessionID: "session", Day: at.Format(time.DateOnly), Facts: []string{"fact"}, CapturedAt: at,
			})
			if err != nil || len(facts) != 1 {
				t.Fatalf("AppendLedger = %+v, %v", facts, err)
			}
			storedFacts, err := memory.PendingLedger(t.Context(), "/repo", 0, 1)
			if err != nil || len(storedFacts) != 1 || !storedFacts[0].CapturedAt.Equal(at) {
				t.Fatalf("Ledger time = %+v, %v", storedFacts, err)
			}
			published, err := memory.Reconcile(t.Context(), agentMemoryPublication(t, "/repo", agentmemory.State{}, facts[0].Sequence, nil, at))
			if err != nil || !published {
				t.Fatalf("Reconcile = %v, %v", published, err)
			}
			storedState, err := memory.State(t.Context(), "/repo")
			if err != nil || !storedState.UpdatedAt.Equal(at) {
				t.Fatalf("Curation time = %+v, %v", storedState, err)
			}

		})
	}
}
