package persistence

import (
	"testing"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

func TestRunStateColumnStoresTheDomainSpelling(t *testing.T) {
	for _, want := range []rundomain.State{rundomain.Running, rundomain.Waiting, rundomain.Finished} {
		got, err := parseRunState(string(want))
		if err != nil || got != want {
			t.Fatalf("parseRunState(%q) = %q, %v; want %q", want, got, err, want)
		}
	}
	for _, invalid := range []string{"", "Running", "terminal", "completed", "finished "} {
		if got, err := parseRunState(invalid); err == nil || got != "" {
			t.Errorf("parseRunState(%q) = %q, %v; want zero/error", invalid, got, err)
		}
	}
}
