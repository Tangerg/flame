package persistence

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestSessionStoresRequireCompleteDurableCapabilities(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	messages := sqlite.NewMessageStore(db)
	compactions, err := NewConversationCompactions(messages, sqlite.NewRunStore(db), func(ctx context.Context, fn func(context.Context) error) error {
		return sqlite.RunInTx(ctx, db, fn)
	})
	if err != nil {
		t.Fatal(err)
	}
	history, err := runs.NewConversationHistory(messages, compactions)
	if err != nil {
		t.Fatal(err)
	}
	complete := SessionStoresConfig{
		Sessions: sqlite.NewSessionStore(db), Transcript: sqlite.NewTranscriptStore(db),
		Interrupts: NewInterruptStore(sqlite.NewInterruptStore(db)), Runs: sqlite.NewRunStore(db),
		ExecutorCheckpoints: NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db)),
		History:             history,
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

func TestSessionWritesRejectUnconstructedPlansBeforeTransaction(t *testing.T) {
	stores := &SessionStores{}
	for name, apply := range map[string]func() error{
		"restore":  func() error { return stores.ApplyRestore(t.Context(), sessions.RestorePlan{}) },
		"fork":     func() error { _, err := stores.ApplyFork(t.Context(), sessions.ForkPlan{}); return err },
		"rollback": func() error { return stores.ApplyRollback(t.Context(), sessions.RollbackPlan{}) },
		"delete":   func() error { return stores.ApplyDelete(t.Context(), sessions.DeletePlan{}) },
		"terminal": func() error { return stores.ApplyTerminal(t.Context(), sessions.TerminalPlan{}) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := apply(); err == nil {
				t.Fatal("unconstructed plan reached persistence")
			}
		})
	}
}
