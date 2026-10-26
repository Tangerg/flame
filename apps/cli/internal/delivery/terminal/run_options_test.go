package terminal

import (
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
)

func defaultRunOptions(t testing.TB) authoring.RunOptions {
	t.Helper()
	options, err := settings.Default().RunOptions()
	if err != nil {
		t.Fatalf("default run options: %v", err)
	}
	return options
}
