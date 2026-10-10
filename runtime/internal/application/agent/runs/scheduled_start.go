package runs

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// StartScheduled projects the captured execution into the shared Run opening.
// Callers can supply a default workspace only when the request owns no cwd;
// instructions, selection and durable identities come solely from the request.
func (c *Coordinator) StartScheduled(ctx context.Context, cmd ScheduledStart) (StartResult, error) {
	if err := cmd.Request.Validate(); err != nil {
		return StartResult{}, fmt.Errorf("%w: %w", ErrInvalidStart, err)
	}
	request := cmd.Request
	execution := request.Execution()
	workspace := execution.CWD()
	if workspace == "" {
		workspace = cmd.DefaultWorkspacePath
	}
	return c.startRoot(ctx, rootStartCommand{
		StartCommand: StartCommand{
			ModelSelection: execution.ModelSelection(),
			Input:          []transcript.ContentBlock{{Kind: transcript.TextContent, Text: execution.Instructions()}},
		},
		scheduled: &request, workspacePath: workspace,
	})
}
