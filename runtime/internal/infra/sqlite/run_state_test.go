package sqlite

import "testing"

func TestRunStateCodecOwnsTheDurableVocabulary(t *testing.T) {
	for _, want := range []runState{runStateRunning, runStateWaiting, runStateTerminal} {
		got, err := parseRunState(want.databaseValue())
		if err != nil || got != want {
			t.Fatalf("parseRunState(%q) = %q, %v; want %q", want.databaseValue(), got, err, want)
		}
	}
	for _, invalid := range []string{"", "Running", "finished", "terminal "} {
		if got, err := parseRunState(invalid); err == nil || got != "" {
			t.Errorf("parseRunState(%q) = %q, %v; want zero/error", invalid, got, err)
		}
	}
}
