package cmd

import (
	json "encoding/json/v2"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
)

func TestShippedExampleIsValidCLIConfiguration(t *testing.T) {
	t.Setenv("FLAME_CLI_PROVIDER", "")
	t.Setenv("FLAME_CLI_MODEL", "")
	path := filepath.Join("..", "..", "..", "config", "config.example.yaml")
	out, _, err := executeCommand(t, instantRuntime(), "", "--config", path, "config", "show")
	if err != nil {
		t.Fatalf("load shipped CLI example: %v", err)
	}
	var got settings.Config
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("example config output: %v\n%s", err, out)
	}
	if got.Provider != "" || got.Model != "" {
		t.Fatalf("example model override = %q/%q, want omitted", got.Provider, got.Model)
	}
}
