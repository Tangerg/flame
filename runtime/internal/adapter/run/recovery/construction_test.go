package recovery

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/goals"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	sqlitebase "github.com/Tangerg/flame/runtime/storage/sqlite"
)

func TestRecoveryRejectsMissingCollaborators(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	valid := Config{
		Sessions: sqlite.NewSessionStore(db), Runs: sqlite.NewRunStore(db),
		Interrupts: persistence.NewInterruptStore(sqlite.NewInterruptStore(db)), Transcript: sqlite.NewTranscriptStore(db),
		Messages:            mustConversationStore(t, sqlite.NewMessageStore(db)),
		GoalRuns:            mustGoalRuns(t, sqlite.NewGoalStore(db)),
		ExecutorCheckpoints: persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db)),
		ModelInvocations:    sqlite.NewModelInvocationStore(db), ToolInvocations: sqlite.NewToolInvocationStore(db),
		ChildRunStarts: sqlite.NewChildRunStartReservationStore(db),
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlitebase.RunInTx(ctx, db, fn)
		},
	}
	for _, test := range []struct {
		name   string
		remove func(*Config)
	}{
		{"sessions", func(c *Config) { c.Sessions = (*sqlite.SessionStore)(nil) }},
		{"runs", func(c *Config) { c.Runs = (*sqlite.RunStore)(nil) }},
		{"interrupts", func(c *Config) { c.Interrupts = (*persistence.InterruptStore)(nil) }},
		{"transcript", func(c *Config) { c.Transcript = (*sqlite.TranscriptStore)(nil) }},
		{"messages", func(c *Config) { c.Messages = (*persistence.ConversationStore)(nil) }},
		{"checkpoints", func(c *Config) { c.ExecutorCheckpoints = (*persistence.ExecutorCheckpointStore)(nil) }},
		{"model invocations", func(c *Config) { c.ModelInvocations = (*sqlite.ModelInvocationStore)(nil) }},
		{"tool invocations", func(c *Config) { c.ToolInvocations = (*sqlite.ToolInvocationStore)(nil) }},
		{"child starts", func(c *Config) { c.ChildRunStarts = (*sqlite.ChildRunStartReservationStore)(nil) }},
		{"transactor", func(c *Config) { c.Tx = nil }},
		{"supplied goal recorder", func(c *Config) { c.GoalRuns = (*goals.RunRecorder)(nil) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.remove(&config)
			store, err := New(config)
			if err == nil || store != nil {
				t.Fatalf("invalid recovery store constructed: %v, %v", store, err)
			}
		})
	}
}
