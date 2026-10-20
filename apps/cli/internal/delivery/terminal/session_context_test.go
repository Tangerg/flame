package terminal

import "testing"

func TestSessionContextRetirementNeverReactivatesAStalePresentation(t *testing.T) {
	stale := newSessionContextLease()
	stale.retire()
	current := newSessionContextLease()

	if current.current(stale) || stale.current(stale) {
		t.Fatal("retiring the last session context epoch reactivated a stale presentation")
	}
}
