package delivery

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestPresentersRejectUnknownDomainEnums(t *testing.T) {
	mustPanic(t, func() { presentItemStatus(transcript.ItemStatus("invalid")) })
	mustPanic(t, func() { presentItemKind(transcript.ItemKind("unknown")) })
	mustPanic(t, func() { presentContent(transcript.ContentBlock{Kind: transcript.ContentKind("invalid")}) })
	mustPanic(t, func() {
		presentQuestion(transcript.Question{Fields: []transcript.QuestionField{{Kind: transcript.QuestionFieldKind("invalid")}}})
	})
	mustPanic(t, func() { presentDelta(nil) })
	mustPanic(t, func() { presentRunStatus(run.Status("invalid")) })
	running := testsupport.MustRestoreRun(run.Snapshot{State: run.Running})
	mustPanic(t, func() { presentOutcome(running) })
	mustPanic(t, func() { presentRunFailure(&run.Failure{Kind: run.FailureKind("invalid")}) })
	mustPanic(t, func() { presentToolFailure(&tool.Failure{Kind: tool.FailureKind("invalid")}) })
	mustPanic(t, func() { presentInterrupts([]transcript.Interrupt{{Kind: run.InterruptKind("invalid")}}) })
}

func TestPresentRunProjectsEveryValidLifecyclePosition(t *testing.T) {
	for _, test := range []struct {
		state  run.State
		status protocol.RunStatus
	}{
		{run.Running, protocol.RunStatusRunning},
		{run.Waiting, protocol.RunStatusWaiting},
		{run.Completed, protocol.RunStatusFinished},
		{run.Canceled, protocol.RunStatusFinished},
		{run.Failed, protocol.RunStatusFinished},
	} {
		t.Run(string(test.state), func(t *testing.T) {
			value := testsupport.MustRestoreRun(run.Snapshot{State: test.state})
			presented := presentRun(value)
			if presented.ID != value.ID() || presented.SessionID != value.SessionID() || presented.Status != test.status || presented.ActiveSegmentID != value.ActiveSegmentID() {
				t.Fatalf("Run projection = %+v, want identity and %s", presented, test.status)
			}
			if (presented.Outcome != nil) != test.state.IsTerminal() {
				t.Fatalf("Run outcome presence changed: %+v", presented)
			}
		})
	}
}

func TestRunOutcomeProjectionIsExhaustive(t *testing.T) {
	tests := []struct {
		domain   run.Outcome
		wire     protocol.RunOutcomeType
		artifact protocol.ArtifactOutcomeType
	}{
		{run.OutcomeCompleted, protocol.OutcomeCompleted, protocol.ArtifactOutcomeCompleted},
		{run.OutcomeCanceled, protocol.OutcomeCanceled, protocol.ArtifactOutcomeCanceled},
		{run.OutcomeTimedOut, protocol.OutcomeTimedOut, protocol.ArtifactOutcomeTimedOut},
		{run.OutcomeFailed, protocol.OutcomeFailed, protocol.ArtifactOutcomeFailed},

		{run.OutcomeLost, protocol.OutcomeLost, protocol.ArtifactOutcomeLost},
	}
	for _, test := range tests {
		t.Run(string(test.domain), func(t *testing.T) {
			outcome := test.domain
			if got := presentOutcome(testsupport.MustRestoreRun(run.Snapshot{Outcome: &outcome})); got.Type != test.wire {
				t.Fatalf("presentOutcome type = %q, want %q", got.Type, test.wire)
			}
			encoded, err := artifactOutcomeType(test.domain)
			if err != nil || encoded != test.artifact {
				t.Fatalf("artifactOutcomeType = (%q, %v), want (%q, nil)", encoded, err, test.artifact)
			}
			decoded, err := portableOutcomeFromArtifact("outcome.type", encoded)
			if err != nil || decoded != test.domain {
				t.Fatalf("portableOutcomeFromArtifact = (%s, %v), want (%s, nil)", decoded, err, test.domain)
			}
		})
	}
}
