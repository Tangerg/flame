package runs

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func installedStartCommand(sessionID, runID, title, instructions, cwd string) StartCommand {
	actor, err := operation.NewActor("11111111-1111-4111-8111-111111111111", testsupport.Digest("release").String(), testsupport.Digest("authority").String(), "schedules")
	if err != nil {
		panic(err)
	}
	return StartCommand{NewSession: &SessionOpening{SessionID: sessionID, RunID: runID, Title: title, CWD: cwd}, Actor: &actor, Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: instructions}}, Acceptance: &CommandIdentity{Key: runID, Fingerprint: testsupport.Digest("opening:" + runID).String()}}
}
func TestInstalledStartProjectsOnlyTheCapturedExecution(t *testing.T) {
	selection := testsupport.MustModelSelection("provider", "captured-model")
	command := installedStartCommand("ses_owned", "run_owned", "Captured title", "Captured instructions", "/captured")
	command.ModelSelection = selection
	control := &fakeExecutionPorts{startRef: ExecutorRef{SessionID: "ses_owned", ExecutorID: "exec_owned"}}
	effects := &fakeEffects{}
	coordinator := newUseCaseCoordinator(&fakeExecutor{}, control, new(fakeRunSessions), effects)
	result, err := coordinator.Start(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	consumeEvents(result.Events)
	if result.SessionID != "ses_owned" || result.RunID != "run_owned" || control.started.ModelSelection != selection || control.started.WorkspaceCWD != "/captured" {
		t.Fatalf("captured execution changed: %+v", control.started)
	}
	if messages := control.started.WorkingContext; len(messages) != 1 || messages[0].Text() != "Captured instructions" {
		t.Fatalf("captured input changed: %+v", messages)
	}
	opening := effects.opening()
	initial, initialized := opening.InitialSession()
	admission, admitted := opening.Admission()
	actor, present := opening.Actor()
	if !initialized || !admitted || !present || actor != *command.Actor || initial.Title() != "Captured title" || initial.Workspace().Path() != "/captured" || initial.Selection() != selection || admission.ModelSelection != selection {
		t.Fatalf("captured opening changed: %+v", opening)
	}
	var recorded []transcript.ContentBlock
	for _, event := range opening.Events() {
		for _, item := range event.Items {
			if item.Kind() == transcript.UserMessage {
				recorded = append(recorded, item.Content()...)
			}
		}
	}
	if len(recorded) != 1 || recorded[0].Text != "Captured instructions" {
		t.Fatalf("durable input changed: %+v", recorded)
	}
}
