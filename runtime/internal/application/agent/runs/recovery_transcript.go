package runs

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func validateRecoveryTranscript(sessionID string, items []transcript.Item) error {
	seen := make(map[string]int, len(items))
	for index, item := range items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("runs: validate recovery transcript Item[%d] %q: %w", index, item.ID(), err)
		}
		if item.SessionID() != sessionID {
			return fmt.Errorf(
				"runs: recovery transcript Item[%d] %q belongs to Session %q, want %q",
				index,
				item.ID(),
				item.SessionID(),
				sessionID,
			)
		}
		if first, duplicate := seen[item.ID()]; duplicate {
			return fmt.Errorf(
				"runs: recovery transcript Item[%d] %q duplicates Item[%d] identity",
				index,
				item.ID(),
				first,
			)
		}
		seen[item.ID()] = index
	}
	return nil
}
