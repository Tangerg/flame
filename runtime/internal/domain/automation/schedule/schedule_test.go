package schedule

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/exactint"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

func TestScheduleDomainValuesOwnAllMutableState(t *testing.T) {
	t.Parallel()

	for _, value := range []any{Schedule{}, Execution{}, Occurrence{}, Claim{}, Acceptance{}, RunRecord{}, RunRequest{}} {
		typ := reflect.TypeOf(value)
		for index := range typ.NumField() {
			field := typ.Field(index)
			if field.IsExported() {
				t.Errorf("%s.%s is exported; domain state must change only through behavior", typ.Name(), field.Name)
			}
		}
	}
}

func TestScheduleNewOwnsInitialLifecycle(t *testing.T) {
	createdAt := time.Date(2026, 8, 29, 8, 30, 0, 123, time.FixedZone("test", 8*60*60))
	scheduled, err := New("sch_daily", Draft{
		Instructions: "review", Cron: "0 9 * * *", Enabled: true,
	}, createdAt)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	wantNext, err := NextRun(scheduled.Cron(), createdAt.UTC())
	if err != nil {
		t.Fatalf("NextRun: %v", err)
	}
	wantCreatedAt := createdAt.UTC().Truncate(durableTimePrecision)
	if scheduled.Revision() != 1 || !scheduled.CreatedAt().Equal(wantCreatedAt) || scheduled.CreatedAt().Location() != time.UTC {
		t.Fatalf("initial lifecycle = revision %d, createdAt %v", scheduled.Revision(), scheduled.CreatedAt())
	}
	if !scheduled.NextRunAt().Equal(wantNext) {
		t.Fatalf("initial cursor = %v, want %v", scheduled.NextRunAt(), wantNext)
	}
}

func TestScheduleRecordRunOwnsDurableTime(t *testing.T) {
	createdAt := time.Date(2026, 8, 29, 8, 0, 0, 987654321, time.UTC)
	scheduled, err := New("sch_record", Draft{Instructions: "review", Cron: "@daily"}, createdAt)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := scheduled.RecordRun(createdAt.Add(-time.Millisecond)); err == nil {
		t.Fatal("RecordRun accepted a completion before Schedule creation")
	}
	ranAt := createdAt.Add(time.Hour + 456*time.Microsecond)
	record, err := scheduled.RecordRun(ranAt)
	if err != nil {
		t.Fatalf("RecordRun: %v", err)
	}
	if record.ScheduleID() != scheduled.ID() || !record.RanAt().Equal(ranAt.UTC().Truncate(durableTimePrecision)) {
		t.Fatalf("RunRecord = (%q, %v)", record.ScheduleID(), record.RanAt())
	}
}

func TestScheduleRestoreRejectsContradictoryLifecycle(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 8, 29, 8, 0, 0, 0, time.UTC)
	base := Snapshot{
		ID: "sch_restore", Instructions: "review", Cron: "@daily",
		CreatedAt: createdAt, Revision: 1,
	}
	tests := []struct {
		name string
		edit func(*Snapshot)
		want error
	}{
		{name: "zero revision", edit: func(s *Snapshot) { s.Revision = 0 }, want: ErrRevisionRequired},
		{name: "inexact revision", edit: func(s *Snapshot) { s.Revision = exactint.Maximum + 1 }, want: ErrRevisionExhausted},
		{name: "missing creation", edit: func(s *Snapshot) { s.CreatedAt = time.Time{} }},
		{name: "enabled without cursor", edit: func(s *Snapshot) { s.Enabled = true }},
		{name: "disabled with cursor", edit: func(s *Snapshot) { s.NextRunAt = createdAt.Add(time.Hour) }},
		{name: "run before creation", edit: func(s *Snapshot) { s.LastRunAt = createdAt.Add(-time.Second) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := base
			test.edit(&snapshot)
			_, err := Restore(snapshot)
			if err == nil {
				t.Fatal("Restore succeeded for contradictory lifecycle")
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("Restore error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestOccurrenceCapturesExecutionAndRejectsEarlyFiring(t *testing.T) {
	createdAt := time.Date(2026, 8, 29, 8, 0, 0, 0, time.UTC)
	scheduled, err := New("sch_hourly", Draft{
		Instructions: "before", Cron: "0 * * * *", Enabled: true,
	}, createdAt)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dueAt := scheduled.NextRunAt()
	if _, err := NewClaim(scheduled, "ses_early", "run_early", dueAt.Add(-time.Second)); err == nil {
		t.Fatal("NewClaim accepted a firing before its due cursor")
	}
	for _, identity := range []string{
		" padded",
		"interior space",
		"hidden\u200bvalue",
		strings.Repeat("界", runtimeidentity.MaximumResourceCharacters+1),
	} {
		if _, err := NewClaim(scheduled, identity, "run_valid", dueAt); err == nil {
			t.Errorf("NewClaim accepted Session identity %q", identity)
		}
		if _, err := NewClaim(scheduled, "ses_valid", identity, dueAt); err == nil {
			t.Errorf("NewClaim accepted Run identity %q", identity)
		}
	}
	claim, err := NewClaim(scheduled, "ses_due", "run_due", dueAt)
	if err != nil {
		t.Fatalf("NewClaim: %v", err)
	}
	occurrence := claim.Occurrence()
	replacement := "after"
	edited, err := scheduled.Edit(Patch{Instructions: &replacement}, scheduled.Revision(), dueAt)
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if edited.Instructions() != replacement || occurrence.Execution().Instructions() != "before" {
		t.Fatalf("execution capture changed with later edit: schedule=%q occurrence=%q", edited.Instructions(), occurrence.Execution().Instructions())
	}
	request := occurrence.RunRequest()
	if err := request.Validate(); err != nil {
		t.Fatalf("occurrence RunRequest: %v", err)
	}
	if request.OccurrenceID() != occurrence.ID() || request.SessionID() != "ses_due" || request.RunID() != "run_due" {
		t.Fatalf("durable RunRequest identities were not preserved")
	}
	manual, err := ManualRunRequest(edited, "ses_manual", "run_manual", dueAt)
	if err != nil {
		t.Fatalf("ManualRunRequest: %v", err)
	}
	if manual.OccurrenceID() != "" || manual.SessionID() != "ses_manual" || manual.RunID() != "run_manual" {
		t.Fatalf("manual request durable identity = (%q, %q, %q)", manual.OccurrenceID(), manual.SessionID(), manual.RunID())
	}
	manualRecord, ok := manual.ManualRecord()
	if !ok || manualRecord.ScheduleID() != edited.ID() || !manualRecord.RanAt().Equal(dueAt) {
		t.Fatalf("manual request Run record = (%+v, %t)", manualRecord, ok)
	}

	snapshot := occurrence.Snapshot()
	malformed := []struct {
		name string
		edit func(*OccurrenceSnapshot)
	}{
		{name: "occurrence belongs to another Schedule", edit: func(value *OccurrenceSnapshot) {
			value.ID = "sch_other:" + strconv.FormatInt(value.DueAt.UnixMilli(), 10)
		}},
		{name: "occurrence due cursor differs", edit: func(value *OccurrenceSnapshot) {
			value.ID = value.ScheduleID + ":" + strconv.FormatInt(value.DueAt.UnixMilli()+1, 10)
		}},
		{name: "malformed Schedule", edit: func(value *OccurrenceSnapshot) {
			value.ScheduleID = "sch_bad id"
		}},
		{name: "malformed occurrence", edit: func(value *OccurrenceSnapshot) {
			value.ID = "sch_hourly:not-a-time"
		}},
	}
	for _, test := range malformed {
		t.Run(test.name, func(t *testing.T) {
			corrupt := snapshot
			test.edit(&corrupt)
			if _, err := RestoreOccurrence(corrupt); err == nil {
				t.Fatal("RestoreOccurrence accepted contradictory identity")
			}
		})
	}
}

func TestScheduleValidate(t *testing.T) {
	base := Draft{Instructions: "do it", Cron: "0 9 * * 1-5", Enabled: true}
	tests := []struct {
		name string
		mut  func(Draft) Draft
		want error // nil = accept
	}{
		{"valid, default model", func(s Draft) Draft { return s }, nil},
		{"valid, paired model", func(s Draft) Draft { s.ModelSelection = mustSelection(t, "anthropic", "claude"); return s }, nil},
		{"missing instructions", func(s Draft) Draft { s.Instructions = ""; return s }, ErrInstructionsRequired},
		{"missing cron", func(s Draft) Draft { s.Cron = ""; return s }, ErrCronRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New("sch_1", tt.mut(base), time.Unix(1, 0))
			if !errors.Is(err, tt.want) {
				t.Fatalf("New() = %v, want %v", err, tt.want)
			}
		})
	}

	bad := base
	bad.Cron = "not a cron"
	if _, err := New("sch_1", bad, time.Unix(1, 0)); !errors.Is(err, ErrInvalidCron) {
		t.Fatalf("garbage cron error = %v, want ErrInvalidCron", err)
	}
}

func TestScheduleApplyPatch(t *testing.T) {
	title := "weekday report"
	emptyCWD := ""
	enabled := false
	sc := mustSchedule(t, Snapshot{
		ID:           "sch_1",
		Title:        "old",
		Instructions: "summarize",
		CWD:          "/work",
		Cron:         "@daily",
		Enabled:      true,
		NextRunAt:    time.Unix(60, 0),
		CreatedAt:    time.Unix(1, 0),
		Revision:     1,
	})

	got, err := sc.Edit(Patch{
		Title:   &title,
		CWD:     &emptyCWD,
		Enabled: &enabled,
	}, sc.Revision(), time.Unix(2, 0))
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if got.Title() != title || got.CWD() != "" || got.Instructions() != sc.Instructions() || got.Cron() != sc.Cron() || got.Enabled() || got.Revision() != 2 {
		t.Fatalf("patched schedule = %+v", got)
	}

	replacement := mustSelection(t, "anthropic", "claude")
	got, err = sc.Edit(Patch{Selection: &replacement}, sc.Revision(), time.Unix(2, 0))
	if err != nil || got.ModelSelection() != replacement {
		t.Fatalf("selection patch = %+v, %v", got.ModelSelection(), err)
	}
	if _, err := sc.Edit(Patch{}, sc.Revision()+1, time.Unix(2, 0)); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale edit error = %v, want ErrRevisionConflict", err)
	}
	exhaustedSnapshot := sc.Snapshot()
	exhaustedSnapshot.Revision = exactint.Maximum
	exhausted := mustSchedule(t, exhaustedSnapshot)
	if _, err := exhausted.Edit(Patch{}, exhausted.Revision(), time.Unix(2, 0)); !errors.Is(err, ErrRevisionExhausted) {
		t.Fatalf("exhausted edit error = %v, want ErrRevisionExhausted", err)
	}
}

func mustSelection(t testing.TB, provider, model string) modelref.Selection {
	t.Helper()
	selection, err := modelref.New(provider, model)
	if err != nil {
		t.Fatalf("modelref.New(%q, %q): %v", provider, model, err)
	}
	return selection
}

func TestScheduleScheduledAfter(t *testing.T) {
	after := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	sc := mustSchedule(t, Snapshot{
		ID: "sch_1", Instructions: "do it", Cron: "0 9 * * 1-5", Enabled: true,
		NextRunAt: after.Add(time.Hour), CreatedAt: after.Add(-time.Hour), Revision: 1,
	})
	got, err := sc.ScheduledAfter(after)
	if err != nil {
		t.Fatalf("ScheduledAfter: %v", err)
	}
	want := time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC)
	if !got.NextRunAt().Equal(want) {
		t.Fatalf("NextRunAt = %v, want %v", got.NextRunAt(), want)
	}

	disabled := mustSchedule(t, Snapshot{
		ID: "sch_2", Instructions: "do it", Cron: "@daily", Enabled: true,
		NextRunAt: want, CreatedAt: after.Add(-time.Hour), Revision: 1,
	})
	disabledFlag := false
	disabled, err = disabled.Edit(Patch{Enabled: &disabledFlag}, disabled.Revision(), after)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	got = disabled
	if err != nil {
		t.Fatalf("ScheduledAfter disabled: %v", err)
	}
	if !got.NextRunAt().IsZero() {
		t.Fatalf("disabled NextRunAt = %v, want zero", got.NextRunAt())
	}
}

func mustSchedule(t testing.TB, snapshot Snapshot) Schedule {
	t.Helper()
	value, err := Restore(snapshot)
	if err != nil {
		t.Fatalf("Restore schedule: %v", err)
	}
	return value
}

func TestValidateCron(t *testing.T) {
	if err := ValidateCron("0 9 * * 1-5"); err != nil {
		t.Errorf("valid 5-field cron rejected: %v", err)
	}
	if err := ValidateCron("@daily"); err != nil {
		t.Errorf("@daily descriptor rejected: %v", err)
	}
	if err := ValidateCron("not a cron"); !errors.Is(err, ErrInvalidCron) {
		t.Errorf("garbage cron error = %v, want ErrInvalidCron", err)
	}
	if err := ValidateCron(""); !errors.Is(err, ErrInvalidCron) {
		t.Errorf("empty cron error = %v, want ErrInvalidCron", err)
	}
}

// TestNextRun: the next firing is strictly after `after` and lands on the
// scheduled minute (weekday 09:00 here).
func TestNextRun(t *testing.T) {
	// A Wednesday 10:00 — next "weekday 9am" is Thursday 09:00.
	after := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	next, err := NextRun("0 9 * * 1-5", after)
	if err != nil {
		t.Fatalf("NextRun: %v", err)
	}
	want := time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("next = %v, want %v", next, want)
	}
	if !next.After(after) {
		t.Errorf("next %v is not strictly after %v", next, after)
	}
}

func TestNextRunInvalid(t *testing.T) {
	if _, err := NextRun("nonsense", time.Now()); !errors.Is(err, ErrInvalidCron) {
		t.Errorf("NextRun error = %v, want ErrInvalidCron", err)
	}
}
