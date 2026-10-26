package sqlite_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	schedule "github.com/Tangerg/flame/plugins/schedules/backend/domain"
	"github.com/Tangerg/flame/plugins/schedules/backend/internal/testsupport"
	"github.com/Tangerg/flame/plugins/schedules/backend/sqlite"
)

func TestCapturedTriggerSurvivesRestartAndCannotBeRewritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedules.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	store := sqlite.NewScheduleStore(db)
	now := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	definition, err := schedule.New("sch_trigger", schedule.Draft{Instructions: "captured", Cron: "@hourly", Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Insert(t.Context(), definition); err != nil {
		t.Fatal(err)
	}
	model := testsupport.ScheduleInvocation()
	model.ID, model.Origin, model.Parent = "original-tool", capability.Model, "run_parent"
	manual, err := definition.ManualRun("ses_manual", "run_manual", now, model)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.QueueManual(t.Context(), manual); err != nil {
		t.Fatal(err)
	}
	automatic := testsupport.ScheduleInvocation()
	claim, err := definition.Claim("ses_due", "run_due", definition.NextRunAt(), automatic)
	if err != nil {
		t.Fatal(err)
	}
	if accepted, err := store.Claim(t.Context(), claim); err != nil || !accepted {
		t.Fatalf("claim: %v, %v", accepted, err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE manual_firings SET captured_request_json='{}' WHERE run_id='run_manual'`); err == nil {
		t.Fatal("captured manual trigger was mutable")
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE schedule_firings SET invocation_json='{}' WHERE run_id='run_due'`); err == nil {
		t.Fatal("captured automatic trigger was mutable")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store = sqlite.NewScheduleStore(db)
	for _, expected := range []schedule.RunRequest{manual, claim.Occurrence().RunRequest()} {
		actual, err := store.CapturedRun(t.Context(), expected.RunID())
		if err != nil || actual.Snapshot() != expected.Snapshot() {
			t.Fatalf("trigger changed after restart: %+v, %v", actual.Snapshot(), err)
		}
	}
}
