package sqlite

import "testing"

func TestScheduleFiringStateCodecIsClosed(t *testing.T) {
	t.Parallel()

	for _, want := range []scheduleFiringState{scheduleFiringPending, scheduleFiringAccepted} {
		got, err := restoreScheduleFiringState(want.databaseValue())
		if err != nil || got != want {
			t.Fatalf("restoreScheduleFiringState(%q) = %q, %v", want, got, err)
		}
	}
	if _, err := restoreScheduleFiringState("unknown"); err == nil {
		t.Fatal("restoreScheduleFiringState accepted an unknown lifecycle state")
	}
}
