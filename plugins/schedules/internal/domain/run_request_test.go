package schedule

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/qimu/portable"
)

func TestCapturedRunRequestRestoresOneExecutionOwner(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	scheduled, err := New("sch_capture", Draft{Instructions: "captured", Cron: "* * * * *", Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	manual, err := scheduled.ManualRun("manual-command", "ses_manual", "run_manual", now, testInvocation())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := scheduled.Claim("ses_cron", "run_cron", scheduled.NextRunAt(), testInvocation())
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []RunRequest{manual, claim.Occurrence().RunRequest()} {
		snapshot := request.Snapshot()
		restored, err := RestoreRunRequest(snapshot)
		if err != nil || restored.Snapshot() != snapshot {
			t.Fatalf("captured request changed on restore: %+v, %v", restored.Snapshot(), err)
		}
		for _, test := range []struct {
			name string
			edit func(*RunRequestSnapshot)
		}{
			{"missing caller", func(v *RunRequestSnapshot) { v.Invocation = portable.Invocation{} }},
			{"missing Session", func(v *RunRequestSnapshot) { v.SessionID = "" }},
			{"missing Run", func(v *RunRequestSnapshot) { v.RunID = "" }},
			{"invalid Session", func(v *RunRequestSnapshot) { v.SessionID = "ses_bad id" }},
			{"invalid Run", func(v *RunRequestSnapshot) { v.RunID = "run_bad id" }},
			{"different Schedule", func(v *RunRequestSnapshot) { v.ScheduleID = "sch_other"; v.OccurrenceID = claim.Occurrence().ID() }},
			{"missing owner", func(v *RunRequestSnapshot) { v.OccurrenceID = ""; v.ManualRanAt = time.Time{} }},
			{"mixed owners", func(v *RunRequestSnapshot) { v.OccurrenceID = claim.Occurrence().ID(); v.ManualRanAt = now }},
		} {
			t.Run(request.RunID()+"/"+test.name, func(t *testing.T) {
				corrupt := snapshot
				test.edit(&corrupt)
				if _, err := RestoreRunRequest(corrupt); err == nil {
					t.Fatal("malformed captured execution was admitted")
				}
			})
		}
	}
	missing := manual.Snapshot()
	missing.CommandID = ""
	if _, err := RestoreRunRequest(missing); err == nil {
		t.Fatal("manual capture without its original receipt command was admitted")
	}
	mixed := claim.Occurrence().RunRequest().Snapshot()
	mixed.CommandID = "manual-command"
	if _, err := RestoreRunRequest(mixed); err == nil {
		t.Fatal("cron capture acquired a manual command owner")
	}
}

func testInvocation() portable.Invocation {
	return portable.Invocation{ID: "schedule-trigger", Origin: extension.Automatic, Parent: "parent-trigger", Target: portable.Target{Scope: "installation", Reference: "12345678-1234-1234-1234-123456789abc"}}
}
