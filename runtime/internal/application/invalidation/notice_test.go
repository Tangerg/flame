package invalidation

import (
	"slices"
	"testing"
)

func TestNoticeConstructorsOwnCallerIdentifiers(t *testing.T) {
	sessionIDs := []string{"session"}
	runIDs := []string{"run"}
	scheduleIDs := []string{"schedule"}
	serverIDs := []string{"server"}

	inSession := InSession(Runs, sessionIDs[0], runIDs...)
	inSessions := InSessions(Sessions, sessionIDs...)
	schedules := ForSchedules(scheduleIDs...)
	servers := ForMCP(serverIDs...)
	runIDs[0] = "changed"
	sessionIDs[0] = "changed"
	scheduleIDs[0] = "changed"
	serverIDs[0] = "changed"

	if !slices.Equal(inSession.RunIDs, []string{"run"}) ||
		!slices.Equal(inSessions.SessionIDs, []string{"session"}) ||
		!slices.Equal(schedules.ScheduleIDs, []string{"schedule"}) ||
		!slices.Equal(servers.ServerIDs, []string{"server"}) {
		t.Fatalf("constructed notices observed caller mutation: %+v %+v %+v %+v", inSession, inSessions, schedules, servers)
	}
}

func TestPublishIsolatesEachCallbackNotice(t *testing.T) {
	shared := []string{"session"}
	notices := []Notice{
		{Resource: Sessions, SessionIDs: shared},
		{Resource: Runs, SessionIDs: shared},
	}
	var received []string
	publisher := Publish(func(notice Notice) {
		received = append(received, notice.SessionIDs[0])
		notice.SessionIDs[0] = "callback-mutated"
	})

	publisher.Notify(notices...)

	if !slices.Equal(received, []string{"session", "session"}) {
		t.Fatalf("callback identifiers = %v, want isolated snapshots", received)
	}
	if shared[0] != "session" || notices[0].SessionIDs[0] != "session" || notices[1].SessionIDs[0] != "session" {
		t.Fatalf("publisher mutated retained notices: shared=%v notices=%+v", shared, notices)
	}
}
