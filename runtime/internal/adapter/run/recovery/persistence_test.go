package recovery

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

func TestCommitRecoveryRejectsUnconstructedPlanBeforeTransaction(t *testing.T) {
	persistence := &Persistence{}
	if err := persistence.CommitRecovery(t.Context(), runs.RecoveryCommit{}); err == nil {
		t.Fatal("unconstructed recovery plan reached persistence")
	}
}
