package delivery

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/protocol"
)

// installationChangedExecution refuses assembly the way execution admission
// does when an installation it depends on changed during assembly.
type installationChangedExecution struct{ executionStub }

func installationChanged() error {
	return fmt.Errorf("execution: installation 940ac827-b431-455b-af4b-e3a170bcfda0 changed during assembly: %w", runs.ErrInstallationChanged)
}

func (installationChangedExecution) StageRoot(context.Context, runs.RootExecutionStart) (runs.ExecutorRef, error) {
	return runs.ExecutorRef{}, installationChanged()
}

func requirePluginChanged(t *testing.T, err error) {
	t.Helper()
	problem := ProjectError(err).Problem()
	if problem.Type != protocol.ErrPluginChanged.Error() {
		t.Fatalf("problem = %+v, want plugin_changed", problem)
	}
	if recovery, _ := RecoveryFor(problem.Type); recovery != protocol.RecoveryPromptUser {
		t.Fatalf("recovery = %q, want the person to decide on a retry", recovery)
	}
	if strings.Contains(problem.Detail, "940ac827") || strings.Contains(problem.Detail, "execution:") {
		t.Fatalf("problem detail leaked the internal cause: %q", problem.Detail)
	}
}

func TestRunStartRacedByInstallationChangeIsTypedAndRetryable(t *testing.T) {
	s, rt := rollbackHarness(t)
	rt.execution = installationChangedExecution{}
	sess, _ := insertSessionFixture(context.Background(), rt.sess, "s", "/w")
	_, _, err := s.StartRun(context.Background(), protocol.StartRunRequest{
		SessionID: sess.ID(),
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "hello"}},
	})
	requirePluginChanged(t, err)
}

func TestRunResumeRacedByInstallationChangeIsTypedAndRetryable(t *testing.T) {
	requirePluginChanged(t, wireRunResumeErr(installationChanged()))
}
