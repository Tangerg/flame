package terminal

import (
	"testing"

	"github.com/Tangerg/flame/cli/internal/application/settings"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func defaultRunOptions(t testing.TB) agent.RunOptions {
	t.Helper()
	options, err := settings.Default().RunOptions()
	if err != nil {
		t.Fatalf("default run options: %v", err)
	}
	return options
}
