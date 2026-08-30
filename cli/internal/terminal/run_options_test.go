package terminal

import (
	"testing"

	"github.com/Tangerg/flame/cli/internal/agent"
	"github.com/Tangerg/flame/cli/internal/settings"
)

func defaultRunOptions(t testing.TB) agent.RunOptions {
	t.Helper()
	options, err := settings.Default().RunOptions()
	if err != nil {
		t.Fatalf("default run options: %v", err)
	}
	return options
}
