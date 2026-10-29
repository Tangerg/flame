package schedule

import (
	"errors"
	"testing"
	"time"
)

func TestScheduleEditPreservesCursorWithoutTimingTransition(t *testing.T) {
	created := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	title, instructions, cwd := "renamed", "new instructions", "/next"
	selection := mustModelSelection("provider", "model")
	enabled := true
	for _, cron := range []string{"@every 1h", "0 * * * *"} {
		for _, after := range []time.Time{created.Add(30 * time.Minute), created.Add(90 * time.Minute)} {
			for _, tc := range []struct {
				name  string
				patch Patch
			}{
				{"empty", Patch{}},
				{"title", Patch{Title: &title}},
				{"instructions", Patch{Instructions: &instructions}},
				{"workspace", Patch{CWD: &cwd}},
				{"model", Patch{Selection: &selection}},
				{"same cron", Patch{Cron: &cron}},
				{"same enablement", Patch{Enabled: &enabled}},
			} {
				t.Run(cron+"/"+after.Format(time.TimeOnly)+"/"+tc.name, func(t *testing.T) {
					original := mustSchedule(t, Snapshot{
						ID: "sch_cursor", Instructions: "review", Cron: cron,
						CreatedAt: created, LastRunAt: created.Add(time.Minute),
						NextRunAt: created.Add(time.Hour), Revision: 2,
					})
					edited, err := original.Edit(tc.patch, original.Revision(), after)
					if err != nil {
						t.Fatal(err)
					}
					if !edited.NextRunAt().Equal(original.NextRunAt()) || !edited.LastRunAt().Equal(original.LastRunAt()) {
						t.Fatalf("metadata edit moved cursor: before=%+v after=%+v", original.Snapshot(), edited.Snapshot())
					}
					if edited.Revision() != original.Revision()+1 {
						t.Fatalf("accepted edit revision = %d", edited.Revision())
					}
				})
			}
		}
	}
}

func TestScheduleEditChangesCursorOnlyForTimingTransition(t *testing.T) {
	created := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	after := created.Add(90 * time.Minute)
	cron, enable, disable := "@every 2h", true, false
	for _, tc := range []struct {
		name    string
		enabled bool
		patch   Patch
		want    time.Time
	}{
		{"new cron", true, Patch{Cron: &cron}, after.Add(2 * time.Hour)},
		{"disable", true, Patch{Enabled: &disable}, time.Time{}},
		{"reenable", false, Patch{Enabled: &enable}, after.Add(time.Hour)},
		{"disabled cron", false, Patch{Cron: &cron}, time.Time{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original, err := New("sch_timing", Draft{Instructions: "review", Cron: "@every 1h", Enabled: tc.enabled}, created)
			if err != nil {
				t.Fatal(err)
			}
			edited, err := original.Edit(tc.patch, original.Revision(), after)
			if err != nil {
				t.Fatal(err)
			}
			if !edited.NextRunAt().Equal(tc.want) || !edited.LastRunAt().Equal(original.LastRunAt()) {
				t.Fatalf("timing transition = %+v, want next %v", edited.Snapshot(), tc.want)
			}
		})
	}
}

func TestScheduleRetainedCursorStillValidatesEdits(t *testing.T) {
	original, err := New("sch_validation", Draft{Instructions: "review", Cron: "@daily", Enabled: true}, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	empty, invalid := "", "invalid"
	for _, tc := range []struct {
		patch Patch
		want  error
	}{
		{Patch{Instructions: &empty}, ErrInstructionsRequired},
		{Patch{Cron: &invalid}, ErrInvalidCron},
	} {
		if _, err := original.Edit(tc.patch, original.Revision(), time.Unix(2, 0)); !errors.Is(err, tc.want) {
			t.Fatalf("invalid edit = %v, want %v", err, tc.want)
		}
	}
}
