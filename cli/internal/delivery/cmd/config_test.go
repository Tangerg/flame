package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/cli/internal/application/settings"
)

func TestConfigurationInheritsTheRuntimeModelByDefault(t *testing.T) {
	t.Setenv("FLAME_CLI_PROVIDER", "")
	t.Setenv("FLAME_CLI_MODEL", "")
	out, _, err := executeCommand(t, instantRuntime(), "", "config", "show")
	if err != nil {
		t.Fatalf("config show: %v", err)
	}
	var got settings.Config
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("config show JSON: %v\n%s", err, out)
	}
	if got.Provider != "" || got.Model != "" {
		t.Fatalf("default model override = %q/%q, want omitted", got.Provider, got.Model)
	}
	if got.Run.MaxTotalTokens != nil || got.Run.MaxSteps != nil || got.Run.MaxBudgetUSD != nil {
		t.Fatalf("default run limits = %+v, want explicit absence", got.Run)
	}
}

func TestRuntimeProviderEnvironmentDoesNotBecomeAClientOverride(t *testing.T) {
	t.Setenv("FLAME_PROVIDER", "anthropic")
	t.Setenv("FLAME_MODEL", "")
	t.Setenv("FLAME_CLI_PROVIDER", "")
	t.Setenv("FLAME_CLI_MODEL", "")
	out, _, err := executeCommand(t, instantRuntime(), "", "config", "show")
	if err != nil {
		t.Fatalf("config show: %v", err)
	}
	var got settings.Config
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("config show JSON: %v\n%s", err, out)
	}
	if got.Provider != "" || got.Model != "" {
		t.Fatalf("Runtime environment leaked into CLI model override = %q/%q", got.Provider, got.Model)
	}
}

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

func TestConfigurationPrecedenceFileEnvironmentFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.yaml")
	if err := os.WriteFile(path, []byte("provider: file-provider\nmodel: file-model\nrun:\n  max-total-tokens: 12000\n  max-steps: 8\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLAME_CLI_MODEL", "environment-model")
	out, _, err := executeCommand(t, instantRuntime(), "", "--config", path, "--max-steps", "12", "config", "show")
	if err != nil {
		t.Fatalf("config show: %v", err)
	}
	var got settings.Config
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("config show JSON: %v\n%s", err, out)
	}
	if got.Provider != "file-provider" || got.Model != "environment-model" || got.Run.MaxTotalTokens == nil || *got.Run.MaxTotalTokens != 12000 || got.Run.MaxSteps == nil || *got.Run.MaxSteps != 12 {
		t.Fatalf("effective settings = %+v", got)
	}
}

func TestProjectConfigurationFollowsTheSelectedWorkspace(t *testing.T) {
	workspace := t.TempDir()
	canonical, err := canonicalWorkspacePath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(canonical, ".flame.yaml")
	if err := os.WriteFile(path, []byte("provider: workspace-provider\nmodel: workspace-model\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := executeCommand(t, instantRuntime(), "", "-C", workspace, "config", "show")
	if err != nil {
		t.Fatal(err)
	}
	var got settings.Config
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Provider != "workspace-provider" || got.Model != "workspace-model" {
		t.Fatalf("workspace settings = %+v", got)
	}

	used, _, err := executeCommand(t, instantRuntime(), "", "-C", workspace, "config", "path")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(used) != path {
		t.Fatalf("workspace configuration path = %q, want %q", strings.TrimSpace(used), path)
	}
}

func TestConfigurationRegistersEnvironmentOnlyKeysForUnmarshal(t *testing.T) {
	t.Setenv("FLAME_CLI_UI_TRANSCRIPT_RETAIN", "77")
	t.Setenv("FLAME_CLI_UI_TOOL_DETAILS", "true")
	t.Setenv("FLAME_CLI_APPROVAL_REMEMBER", "project")
	t.Setenv("FLAME_CLI_RUN_MAX_TOTAL_TOKENS", "24000")
	t.Setenv("FLAME_CLI_RUN_MAX_STEPS", "9")
	t.Setenv("FLAME_CLI_RUN_MAX_BUDGET_USD", "1.25")
	out, _, err := executeCommand(t, instantRuntime(), "", "config", "show")
	if err != nil {
		t.Fatal(err)
	}
	var got settings.Config
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.UI.TranscriptRetain != 77 || !got.UI.ToolDetails || got.Approval.Remember != "project" ||
		got.Run.MaxTotalTokens == nil || *got.Run.MaxTotalTokens != 24000 ||
		got.Run.MaxSteps == nil || *got.Run.MaxSteps != 9 ||
		got.Run.MaxBudgetUSD == nil || *got.Run.MaxBudgetUSD != 1.25 {
		t.Fatalf("environment settings = %+v", got)
	}
}

func TestConfigurationRunLimitPrecedenceIsFileEnvironmentFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.yaml")
	if err := os.WriteFile(path, []byte("run:\n  max-total-tokens: 12000\n  max-steps: 4\n  max-budget-usd: 0.5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLAME_CLI_RUN_MAX_TOTAL_TOKENS", "24000")
	t.Setenv("FLAME_CLI_RUN_MAX_STEPS", "8")
	t.Setenv("FLAME_CLI_RUN_MAX_BUDGET_USD", "1.5")
	out, _, err := executeCommand(t, instantRuntime(), "", "--config", path, "--max-steps", "12", "config", "show")
	if err != nil {
		t.Fatal(err)
	}
	var got settings.Config
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Run.MaxTotalTokens == nil || *got.Run.MaxTotalTokens != 24000 ||
		got.Run.MaxSteps == nil || *got.Run.MaxSteps != 12 ||
		got.Run.MaxBudgetUSD == nil || *got.Run.MaxBudgetUSD != 1.5 {
		t.Fatalf("effective run limits = %+v", got.Run)
	}
}

func TestConfigurationAcceptsRepeatablePluginDirectories(t *testing.T) {
	out, _, err := executeCommand(t, instantRuntime(), "", "--plugin-dir", "/plugins/one", "--plugin-dir", "/plugins/two", "config", "show")
	if err != nil {
		t.Fatal(err)
	}
	var got settings.Config
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Plugins.Directories) != 2 || got.Plugins.Directories[0] != "/plugins/one" || got.Plugins.Directories[1] != "/plugins/two" {
		t.Fatalf("plugin directories = %v", got.Plugins.Directories)
	}
}

func TestConfigurationMergesPartialKeyOverridesWithDefaultActions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.yaml")
	if err := os.WriteFile(path, []byte("keys:\n  sessions: [g s]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := executeCommand(t, instantRuntime(), "", "--config", path, "config", "show")
	if err != nil {
		t.Fatal(err)
	}
	var got settings.Config
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if bindings := got.Keys[settings.ActionSessions]; len(bindings) != 1 || bindings[0] != "g s" {
		t.Fatalf("session bindings = %v", bindings)
	}
	if bindings := got.Keys[settings.ActionShortcuts]; len(bindings) != 1 || bindings[0] != "ctrl+x" {
		t.Fatalf("default shortcut bindings = %v", bindings)
	}
}

func TestConfigurationRejectsInvalidValuesAndMissingExplicitFile(t *testing.T) {
	for _, value := range []string{"0", "-1", "not-a-number"} {
		if _, _, err := executeCommand(t, instantRuntime(), "", "--max-steps="+value, "config", "show"); err == nil {
			t.Fatalf("run step limit %q was accepted", value)
		}
	}
	for _, value := range []string{"0", "NaN", "+Inf"} {
		if _, _, err := executeCommand(t, instantRuntime(), "", "--max-budget-usd="+value, "config", "show"); err == nil {
			t.Fatalf("run budget limit %q was accepted", value)
		}
	}
	zeroConfig := filepath.Join(t.TempDir(), "zero-limit.yaml")
	if err := os.WriteFile(zeroConfig, []byte("run:\n  max-budget-usd: 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := executeCommand(t, instantRuntime(), "", "--config", zeroConfig, "config", "show"); err == nil {
		t.Fatal("configuration numeric zero sentinel was accepted")
	}
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	if _, _, err := executeCommand(t, instantRuntime(), "", "--config", missing, "config", "show"); err == nil {
		t.Fatal("missing explicit configuration was ignored")
	}
}

func TestConfigurationRejectsUnknownKeys(t *testing.T) {
	for name, content := range map[string]string{
		"top level": "unknown-setting: value\n",
		"nested":    "ui:\n  transcript-retian: 80\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := executeCommand(t, instantRuntime(), "", "--config", path, "config", "show"); err == nil {
				t.Fatal("unknown configuration key was ignored")
			}
		})
	}
}

func TestCompletionGenerationDoesNotDependOnConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(path, []byte("unknown: value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := executeCommand(t, instantRuntime(), "", "--config", path, "completion", "bash")
	if err != nil {
		t.Fatalf("completion generation read configuration: %v", err)
	}
	if !strings.Contains(out, "bash completion") {
		t.Fatalf("completion output is incomplete:\n%s", out)
	}
}
