package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
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

func TestLoadSelectsOnlyConfigYAMLInDirectoryPrecedenceOrder(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(first, "config.json"),
		[]byte(`{"provider":"openai","model":"wrong-format"}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(second, "config.yaml"),
		[]byte("provider: anthropic\nmodel: expected-yaml\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLAME_PROVIDER", "")
	t.Setenv("FLAME_MODEL", "")

	settings, err := Load([]string{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if settings.Provider != "anthropic" || settings.Model != "expected-yaml" {
		t.Fatalf("settings = %+v, want config.yaml from second search directory", settings)
	}
}

func TestLoadRejectsUnboundedOrNonRegularConfigFile(t *testing.T) {
	t.Run("oversized", func(t *testing.T) {
		directory := t.TempDir()
		path := filepath.Join(directory, "config.yaml")
		content := "provider: anthropic\n"
		content += strings.Repeat(" ", int(maximumRuntimeConfigBytes)-len(content))
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("FLAME_PROVIDER", "")
		if _, err := Load([]string{directory}); err != nil {
			t.Fatalf("Load maximum-sized file: %v", err)
		}

		file, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Truncate(maximumRuntimeConfigBytes + 1); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := Load([]string{directory}); !errors.Is(err, fileinput.ErrTooLarge) {
			t.Fatalf("Load oversized error = %v, want ErrTooLarge", err)
		}
	})

	t.Run("directory", func(t *testing.T) {
		directory := t.TempDir()
		if err := os.Mkdir(filepath.Join(directory, "config.yaml"), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := Load([]string{directory}); !errors.Is(err, fileinput.ErrNotRegular) {
			t.Fatalf("Load directory error = %v, want ErrNotRegular", err)
		}
	})
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
