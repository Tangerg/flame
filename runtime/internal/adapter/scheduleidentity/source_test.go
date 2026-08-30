package scheduleidentity

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/schedule"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

func TestSourceProducesNamespacedFreshIdentities(t *testing.T) {
	t.Parallel()

	source := Source{}
	firstSchedule, secondSchedule := source.NewScheduleID(), source.NewScheduleID()
	firstSession, secondSession := source.NewSessionID(), source.NewSessionID()
	firstRun, secondRun := source.NewRunID(), source.NewRunID()
	for name, check := range map[string]bool{
		"schedule prefix": strings.HasPrefix(firstSchedule, schedule.IDPrefix),
		"session prefix":  strings.HasPrefix(firstSession, session.IDPrefix),
		"run prefix":      strings.HasPrefix(firstRun, runs.NewRunID("")),
		"schedule fresh":  firstSchedule != secondSchedule,
		"session fresh":   firstSession != secondSession,
		"run fresh":       firstRun != secondRun,
	} {
		if !check {
			t.Errorf("identity invariant failed: %s", name)
		}
	}
}
