package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPIKeyInputFormattingNeverRevealsTheCredential(t *testing.T) {
	const secret = "sk-config-must-not-escape"
	for _, key := range []APIKeyInput{FileAPIKey(secret), EnvironmentAPIKey(secret)} {
		settings := Settings{Provider: "anthropic", APIKey: key}
		for _, format := range []string{"%s", "%v", "%+v", "%#v", "%q"} {
			if rendered := fmt.Sprintf(format, settings); strings.Contains(rendered, secret) {
				t.Fatalf("format %q revealed configuration credential material", format)
			}
		}
	}
}

func TestLoadUsesOnlyExplicitAbsoluteSearchDirectories(t *testing.T) {
	if _, err := Load([]string{"relative/config"}); err == nil {
		t.Fatal("Load accepted a relative config search directory")
	}

	directory := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(directory, "config.yaml"),
		[]byte("provider: anthropic\nmodel: explicit-model\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLAME_PROVIDER", "")
	t.Setenv("FLAME_MODEL", "")
	settings, err := Load([]string{directory})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if settings.Provider != "anthropic" || settings.Model != "explicit-model" {
		t.Fatalf("settings = %+v, want explicit config file values", settings)
	}
	if !settings.ToolResultOffload.Enabled || settings.ToolResultOffload.Threshold != DefaultToolResultOffloadThreshold {
		t.Fatalf("default Tool-result offload = %+v", settings.ToolResultOffload)
	}
}

func TestLoadToolResultOffloadUsesExplicitEnablement(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.yaml")
	if err := os.WriteFile(path, []byte("provider: anthropic\ntoolResultOffload:\n  enabled: false\n  threshold: 1234\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLAME_PROVIDER", "")
	t.Setenv("FLAME_TOOLRESULTOFFLOAD_ENABLED", "")
	t.Setenv("FLAME_TOOLRESULTOFFLOAD_THRESHOLD", "")
	settings, err := Load([]string{directory})
	if err != nil {
		t.Fatal(err)
	}
	if settings.ToolResultOffload.Enabled || settings.ToolResultOffload.Threshold != 1234 {
		t.Fatalf("Tool-result offload = %+v", settings.ToolResultOffload)
	}

	if err := os.WriteFile(path, []byte("provider: anthropic\ntoolResultOffload:\n  enabled: false\n  threshold: 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load([]string{directory}); err == nil {
		t.Fatal("numeric zero was accepted as an implicit offload switch")
	}
}

func TestLoadRejectsUnknownConfigurationFields(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(directory, "config.yaml"),
		[]byte("provider: anthropic\nsandbox:\n  shel: true\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLAME_PROVIDER", "")

	_, err := Load([]string{directory})
	if err == nil || !strings.Contains(err.Error(), "sandbox") || !strings.Contains(err.Error(), "shel") {
		t.Fatalf("Load unknown field error = %v", err)
	}
}
