package runs

import (
	"strconv"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
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
	manual, err := scheduled.ManualRun("ses_owned", "run_owned", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	occurrence, err := schedule.RestoreOccurrence(schedule.OccurrenceSnapshot{
		ID: "sch_owned:" + strconv.FormatInt(now.UnixMilli(), 10), Execution: scheduled.Execution().Snapshot(),
		FiredAt: now, NextRunAt: now.Add(24 * time.Hour),
		SessionID: "ses_owned", RunID: "run_owned",
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
			result, err := coordinator.StartAutomation(t.Context(), automationRequest(test.request, "/default"))
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
			captured, present := opening.Automation()
			if !initialized || !admitted || !present || captured != automationRequest(test.request, "/default") ||
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

func automationRequest(request schedule.RunRequest, cwd string) AutomationStart {
	execution := request.Execution()
	if execution.CWD() != "" {
		cwd = execution.CWD()
	}
	return AutomationStart{Origin: AutomationOrigin{InstallationID: "11111111-1111-4111-8111-111111111111", ReleaseDigest: testsupport.Digest("release").String(), ServerName: "schedules"}, SessionID: request.SessionID(), RunID: request.RunID(), Title: execution.Title(), Instructions: execution.Instructions(), CWD: cwd, ModelSelection: execution.ModelSelection()}
}
