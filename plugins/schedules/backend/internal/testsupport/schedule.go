package testsupport

import (
	"strconv"
	"time"

	"github.com/Tangerg/flame/runtime/capability"

	schedule "github.com/Tangerg/flame/plugins/schedules/backend/domain"
)

func ScheduleInvocation() capability.Invocation {
	return capability.Invocation{ID: "schedule-trigger", Origin: capability.Automatic, Parent: "parent-trigger", Target: capability.Target{Scope: "installation", Reference: "12345678-1234-1234-1234-123456789abc"}}
}

// MustOccurrenceRunRequest returns the launch input of one durable cron
// occurrence of a minimal titled Schedule, and panics when the fixture is invalid.
func MustOccurrenceRunRequest(scheduleID string, dueAt time.Time, sessionID, runID string) schedule.RunRequest {
	dueAt = dueAt.UTC()
	occurrence, err := schedule.RestoreOccurrence(schedule.OccurrenceSnapshot{
		ID:        scheduleID + ":" + strconv.FormatInt(dueAt.UnixMilli(), 10),
		Execution: schedule.ExecutionSnapshot{Title: "Scheduled", Instructions: "scheduled"},
		FiredAt:   dueAt, NextRunAt: dueAt.Add(24 * time.Hour),
		SessionID: sessionID, RunID: runID,
		Invocation: ScheduleInvocation(),
	})
	if err != nil {
		panic(err)
	}
	return occurrence.RunRequest()
}
