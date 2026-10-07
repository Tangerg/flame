package testsupport

import (
	"strconv"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
)

// MustOccurrenceRunRequest returns the launch input of one durable cron
// occurrence of a minimal titled Schedule, and panics when the fixture is invalid.
func MustOccurrenceRunRequest(scheduleID string, dueAt time.Time, sessionID, runID string) schedule.RunRequest {
	dueAt = dueAt.UTC()
	occurrence, err := schedule.RestoreOccurrence(schedule.OccurrenceSnapshot{
		ID:        scheduleID + ":" + strconv.FormatInt(dueAt.UnixMilli(), 10),
		Execution: schedule.ExecutionSnapshot{Title: "Scheduled", Instructions: "scheduled"},
		FiredAt:   dueAt, NextRunAt: dueAt.Add(24 * time.Hour),
		SessionID: sessionID, RunID: runID,
	})
	if err != nil {
		panic(err)
	}
	return occurrence.RunRequest()
}
