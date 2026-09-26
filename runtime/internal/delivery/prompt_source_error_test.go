package delivery

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestPromptBudgetFailuresAreActionableWithoutSourceContent(t *testing.T) {
	for _, test := range []struct {
		name    string
		project func(error) error
	}{
		{"runs.start", wireRunStartErr},
		{"agentDocs.list", wireWorkspaceError},
		{"schedules.runNow", func(err error) error { return mapScheduleErr(err, "sch_test") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.project(fmt.Errorf("private AGENTS.md content: %w", workspaceapp.ErrPromptSourceTooLarge))
			if !errors.Is(err, protocol.ErrPromptSourceTooLarge) {
				t.Fatalf("wire error = %v, want prompt_source_too_large", err)
			}
			failure, ok := errors.AsType[*Failure](err)
			if !ok {
				t.Fatalf("wire error = %T, want a public failure", err)
			}
			problem := failure.Problem()
			recovery, registered := RecoveryFor(problem.Type)
			if problem.Type != "prompt_source_too_large" || !registered || recovery != protocol.RecoveryPromptUser || !strings.Contains(problem.Detail, "shorten") {
				t.Fatalf("problem = %+v, want actionable source correction", problem)
			}
			if strings.Contains(problem.Detail, "private") {
				t.Fatalf("problem leaked source content: %+v", problem)
			}
		})
	}
}
