package runs

import (
	"strconv"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/operation"

	schedule "github.com/Tangerg/flame/plugins/schedules/backend/domain"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestScheduledStartProjectsOnlyTheCapturedExecution(t *testing.T) {
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	selection := testsupport.MustModelSelection("provider", "captured-model")
	scheduled, err := schedule.New("sch_owned", schedule.Draft{
		Title: "Captured title", Instructions: "Captured instructions",
		CWD: "/captured", ModelSelection: selection, Cron: "@daily",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	manual, err := scheduled.ManualRun("ses_owned", "run_owned", now.Add(time.Second), testsupport.ScheduleInvocation())
	if err != nil {
		t.Fatal(err)
	}
	occurrence, err := schedule.RestoreOccurrence(schedule.OccurrenceSnapshot{
		ID: "sch_owned:" + strconv.FormatInt(now.UnixMilli(), 10), Execution: scheduled.Execution().Snapshot(),
		FiredAt: now, NextRunAt: now.Add(24 * time.Hour),
		SessionID: "ses_owned", RunID: "run_owned", Invocation: testsupport.ScheduleInvocation(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		request schedule.RunRequest
	}{
		{name: "manual", request: manual},
		{name: "cron", request: occurrence.RunRequest()},
	} {
		t.Run(test.name, func(t *testing.T) {
			control := &fakeExecutionPorts{startRef: ExecutorRef{SessionID: "ses_owned", ExecutorID: "exec_owned"}}
			effects := &fakeEffects{}
			coordinator := newUseCaseCoordinator(&fakeExecutor{}, control, new(fakeRunSessions), effects)
			result, err := coordinator.Start(t.Context(), openingCommand(test.request, "/default"))
			if err != nil {
				t.Fatalf("start from captured request: %v", err)
			}
			consumeEvents(result.Events)
			if result.SessionID != test.request.SessionID() || result.RunID != test.request.RunID() {
				t.Fatalf("start changed captured identity: %+v", result)
			}
			if control.started.ModelSelection != selection || control.started.WorkspaceCWD != "/captured" {
				t.Fatalf("execution changed captured configuration: %+v", control.started)
			}
			messages := control.started.WorkingContext
			if len(messages) != 1 || messages[0].Text() != scheduled.Instructions() {
				t.Fatalf("execution changed captured instructions: %+v", messages)
			}
			opening := effects.opening()
			initial, initialized := opening.InitialSession()
			admission, admitted := opening.Admission()
			captured, present := opening.Actor()
			if !initialized || !admitted || !present || captured != *openingCommand(test.request, "/default").Actor ||
				initial.Title() != scheduled.Title() || initial.Workspace().Path() != "/captured" ||
				initial.Selection() != selection || admission.ModelSelection != selection {
				t.Fatalf("durable opening changed captured execution: %+v", opening)
			}
			var recorded []transcript.ContentBlock
			for _, event := range opening.Events() {
				for _, item := range event.Items {
					if item.Kind() == transcript.UserMessage {
						recorded = append(recorded, item.Content()...)
					}
				}
			}
			if len(recorded) != 1 || recorded[0].Text != scheduled.Instructions() {
				t.Fatalf("durable input changed captured instructions: %+v", recorded)
			}
		})
	}
}

func openingCommand(request schedule.RunRequest, cwd string) StartCommand {
	execution := request.Execution()
	if execution.CWD() != "" {
		cwd = execution.CWD()
	}
	actor, err := operation.NewActor("11111111-1111-4111-8111-111111111111", testsupport.Digest("release").String(), testsupport.Digest("authority").String(), "schedules")
	if err != nil {
		panic(err)
	}
	return StartCommand{NewSession: &SessionOpening{SessionID: request.SessionID(), RunID: request.RunID(), Title: execution.Title(), CWD: cwd}, Actor: &actor, Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: execution.Instructions()}}, ModelSelection: execution.ModelSelection(), Acceptance: &CommandIdentity{Key: request.RunID(), Fingerprint: testsupport.Digest("opening:" + request.RunID()).String()}}
}
