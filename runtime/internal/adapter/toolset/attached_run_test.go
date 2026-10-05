package toolset

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

// attachedRun is the Run scope every Tool resolution and call executes under.
func attachedRun(t *testing.T) context.Context {
	t.Helper()
	workspace := t.TempDir()
	return executionctx.WithScope(t.Context(), runs.ExecutionScope{
		SessionID: "session-test", CWD: workspace, WorkspaceCWD: workspace,
	})
}
