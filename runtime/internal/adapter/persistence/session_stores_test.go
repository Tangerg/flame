package persistence

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestSessionStoresRequireCompleteDurableCapabilities(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	complete := SessionStoresConfig{
		Sessions: sqlite.NewSessionStore(db), Transcript: sqlite.NewTranscriptStore(db),
		Interrupts: NewInterruptStore(sqlite.NewInterruptStore(db)), Runs: sqlite.NewRunStore(db),
		ExecutorCheckpoints: NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db)),
		History:             runs.NewConversationHistory(sqlite.NewMessageStore(db), nil),
		Plan:                sqlite.NewPlanStore(db),
		ApprovalRules:       sqlite.NewApprovalRuleStore(db),
		PermissionModes:     sqlite.NewPermissionModeStore(db),
		ToolResults:         sqlite.NewToolResultStore(db),
		ChildRunStarts:      sqlite.NewChildRunStartReservationStore(db),
		Goals:               sqlite.NewGoalStore(db),
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	}
	if _, err := NewSessionStores(complete); err != nil {
		t.Fatalf("complete persistence: %v", err)
	}
	for name, remove := range map[string]func(*SessionStoresConfig){
		"plan":         func(cfg *SessionStoresConfig) { cfg.Plan = nil },
		"goal":         func(cfg *SessionStoresConfig) { cfg.Goals = nil },
		"typed goal":   func(cfg *SessionStoresConfig) { cfg.Goals = (*sqlite.GoalStore)(nil) },
		"tool results": func(cfg *SessionStoresConfig) { cfg.ToolResults = nil },
		"transaction":  func(cfg *SessionStoresConfig) { cfg.Tx = nil },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := complete
			remove(&cfg)
			if stores, err := NewSessionStores(cfg); err == nil || stores != nil {
				t.Fatalf("incomplete persistence = %v, %v", stores, err)
			}
		})
	}
}
