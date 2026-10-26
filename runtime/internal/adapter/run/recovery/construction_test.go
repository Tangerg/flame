package recovery

import (
	"context"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/goals"
)

func TestRecoveryRejectsMissingCollaborators(t *testing.T) {
	db, err := persistencefixture.Open(t, t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	valid := Config{
		Sessions: db.Sessions, Runs: db.Runs,
		Interrupts: db.Interrupts, Transcript: db.Transcript,
		Messages:            mustConversationStore(t, db.ChatHistory),
		GoalRuns:            mustGoalRuns(t, db.Goals),
		ExecutorCheckpoints: db.ExecutorCheckpoints,
		ModelInvocations:    db.ModelInvocations, ToolInvocations: db.ToolInvocations,
		ChildRunStarts: db.ChildRunStarts,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return db.Transactor(ctx, fn)
		},
	}
	for _, test := range []struct {
		name   string
		remove func(*Config)
	}{
		{"sessions", func(c *Config) { c.Sessions = (*persistence.SessionStore)(nil) }},
		{"runs", func(c *Config) { c.Runs = (*persistence.RunStore)(nil) }},
		{"interrupts", func(c *Config) { c.Interrupts = (*persistence.InterruptStore)(nil) }},
		{"transcript", func(c *Config) { c.Transcript = (*persistence.TranscriptStore)(nil) }},
		{"messages", func(c *Config) { c.Messages = (*persistence.ConversationStore)(nil) }},
		{"checkpoints", func(c *Config) { c.ExecutorCheckpoints = (*persistence.ExecutorCheckpointStore)(nil) }},
		{"model invocations", func(c *Config) { c.ModelInvocations = (*persistence.ModelInvocationStore)(nil) }},
		{"tool invocations", func(c *Config) { c.ToolInvocations = (*persistence.ToolInvocationStore)(nil) }},
		{"child starts", func(c *Config) { c.ChildRunStarts = (*persistence.ChildRunStartReservationStore)(nil) }},
		{"transactor", func(c *Config) { c.Tx = nil }},
		{"goal recorder", func(c *Config) { c.GoalRuns = nil }},
		{"typed nil goal recorder", func(c *Config) { c.GoalRuns = (*goals.RunRecorder)(nil) }},
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
