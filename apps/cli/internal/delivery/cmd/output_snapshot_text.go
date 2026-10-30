package cmd

import (
	"fmt"
	"io"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

// Saved transcripts can contain multiple root Runs and their descendants,
// so they have no live Run scope.
func writeSessionTranscript(w io.Writer, snapshot conversation.SessionSnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("render session transcript: %w", err)
	}
	renderer := newTextRenderer(w)
	renderer.scope.ensureMembers()
	for _, run := range snapshot.Runs {
		renderer.scope.members[run.ID] = run.Lineage
	}
	for _, block := range snapshot.Transcript {
		renderer.finish(block)
	}
	return renderer.Close()
}
