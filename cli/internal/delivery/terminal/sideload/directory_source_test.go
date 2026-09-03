package sideload

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/extensions"
	"github.com/Tangerg/flame/cli/internal/delivery/terminal"
)

func writePlugin(t *testing.T, root, directory string, declared pluginManifest, executable string) {
	t.Helper()
	pluginDirectory := filepath.Join(root, directory)
	if err := os.MkdirAll(pluginDirectory, 0o750); err != nil {
		t.Fatal(err)
	}
	if executable != "" {
		writeExecutable(t, filepath.Join(pluginDirectory, declared.Entry), executable)
	}
	encoded, err := json.Marshal(declared)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDirectory, manifestName), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func validManifest(id string) pluginManifest {
	return pluginManifest{
		SchemaVersion: manifestSchemaVersion, ID: id, Version: "1.2.3", APIVersion: extensions.HostAPIVersion,
		Capabilities: []string{"terminal.commands"}, Entry: "plugin",
		Contributes: manifestContributions{Commands: []commandManifest{{Name: "hello", Title: "say hello", Arguments: "required"}}},
	}
}

func TestDirectorySourceDiscoversValidPluginsAndIsolatesMalformedNeighbors(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "good", validManifest("test.good"), "not executed")
	broken := filepath.Join(root, "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, manifestName), []byte(`{"schemaVersion":`), 0o600); err != nil {
		t.Fatal(err)
	}

	discovered, err := New([]string{root}).Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered.Plugins) != 1 || discovered.Plugins[0].ID != "test.good" {
		t.Fatalf("plugins = %+v", discovered.Plugins)
	}
	if len(discovered.Issues) != 1 || !strings.Contains(discovered.Issues[0].Error(), "broken") {
		t.Fatalf("issues = %+v", discovered.Issues)
	}
}

func TestDirectorySourceRejectsNonRegularManifest(t *testing.T) {
	root := t.TempDir()
	pluginDirectory := filepath.Join(root, "invalid")
	if err := os.MkdirAll(filepath.Join(pluginDirectory, manifestName), 0o700); err != nil {
		t.Fatal(err)
	}

	discovered, err := New([]string{root}).Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered.Plugins) != 0 || len(discovered.Issues) != 1 || !strings.Contains(discovered.Issues[0].Error(), "not a regular file") {
		t.Fatalf("discovery = %+v", discovered)
	}
}

func TestDirectorySourceDeduplicatesCanonicalPluginDirectories(t *testing.T) {
	root := t.TempDir()
	pluginDirectory := filepath.Join(root, "good")
	writePlugin(t, root, "good", validManifest("test.good"), "not executed")
	discovered, err := New([]string{root, filepath.Join(root, "."), pluginDirectory, pluginDirectory}).Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered.Issues) != 0 || len(discovered.Plugins) != 1 || discovered.Plugins[0].ID != "test.good" {
		t.Fatalf("discovery = %+v", discovered)
	}
}

func TestManifestEntryCannotEscapePluginDirectory(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	writeExecutable(t, outside, "outside")
	declared := validManifest("test.escape")
	declared.Entry = "../outside"
	writePlugin(t, root, "escape", declared, "")

	discovered, err := New([]string{root}).Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered.Plugins) != 0 || len(discovered.Issues) != 1 || !strings.Contains(discovered.Issues[0].Error(), "unsafe path") {
		t.Fatalf("discovery = %+v", discovered)
	}
}

func TestSideloadedPluginMustDeclareCommandsCapability(t *testing.T) {
	root := t.TempDir()
	declared := validManifest("test.denied")
	declared.Capabilities = []string{}
	writePlugin(t, root, "denied", declared, "not executed")
	discovered, err := New([]string{root}).Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	registry := new(extensions.Registry)
	extensionHost, err := extensions.NewHost(registry)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = extensionHost.Close() }()
	results, err := extensionHost.Activate(discovered.Plugins)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Phase != extensions.PluginFailed || !strings.Contains(results[0].Err.Error(), "terminal.commands") {
		t.Fatalf("activation = %+v", results)
	}
	if commands := registry.Values(terminal.SlashCommands); len(commands) != 0 {
		t.Fatalf("denied plugin registered commands: %+v", commands)
	}
}

func TestSideloadedCommandRejectsAnInvalidArgumentMode(t *testing.T) {
	root := t.TempDir()
	declared := validManifest("test.arguments")
	declared.Contributes.Commands[0].Arguments = "sometimes"
	writePlugin(t, root, "arguments", declared, "not executed")
	discovered, err := New([]string{root}).Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered.Plugins) != 0 || len(discovered.Issues) != 1 || !strings.Contains(discovered.Issues[0].Error(), "invalid arguments mode") {
		t.Fatalf("discovery = %+v", discovered)
	}
}

func TestSideloadedCommandTimeoutDistinguishesAbsenceFromExplicitZero(t *testing.T) {
	t.Parallel()
	if timeout, err := (commandTimeoutDeclaration{}).Resolve("hello"); err != nil || timeout != defaultCommandTimeout {
		t.Fatalf("default timeout = (%s, %v), want %s", timeout, err, defaultCommandTimeout)
	}
	for _, seconds := range []int{1, int(maxCommandTimeout.Seconds())} {
		declaration := commandTimeoutDeclaration{present: true, seconds: seconds}
		if timeout, err := declaration.Resolve("hello"); err != nil || timeout != time.Duration(seconds)*time.Second {
			t.Fatalf("timeout %d = (%s, %v)", seconds, timeout, err)
		}
	}
	for _, seconds := range []int{0, -1, int(maxCommandTimeout.Seconds()) + 1} {
		if _, err := (commandTimeoutDeclaration{present: true, seconds: seconds}).Resolve("hello"); err == nil {
			t.Fatalf("timeout %d was accepted", seconds)
		}
	}
	if strconv.IntSize == 64 {
		// This value wraps to a positive 512ns if it is multiplied by one
		// second before its declared unit is bounded.
		const wrappingSeconds int64 = 20_211_507_185_753_197
		if _, err := (commandTimeoutDeclaration{present: true, seconds: int(wrappingSeconds)}).Resolve("hello"); err == nil {
			t.Fatalf("overflowing timeout %d was accepted", wrappingSeconds)
		}
	}
	if _, err := (commandTimeoutDeclaration{seconds: 1}).Resolve("hello"); err == nil {
		t.Fatal("absent timeout carrying seconds was accepted")
	}
}

func TestManifestRejectsExplicitZeroCommandTimeout(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	declared := validManifest("test.zero-timeout")
	declared.Contributes.Commands[0].Timeout = commandTimeoutDeclaration{present: true}
	writePlugin(t, root, "zero-timeout", declared, "not executed")
	discovered, err := New([]string{root}).Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered.Plugins) != 0 || len(discovered.Issues) != 1 || !strings.Contains(discovered.Issues[0].Error(), "timeout") {
		t.Fatalf("discovery = %+v", discovered)
	}
}

func TestManifestRejectsNullCommandTimeout(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	directory := filepath.Join(root, "null-timeout")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schemaVersion":2,"id":"test.null-timeout","version":"1.0.0","apiVersion":1,"requires":[],"capabilities":["terminal.commands"],"entry":"plugin","contributes":{"commands":[{"name":"hello","title":"say hello","arguments":"none","timeoutSeconds":null}]}}`
	if err := os.WriteFile(filepath.Join(directory, manifestName), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	discovered, err := New([]string{root}).Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered.Plugins) != 0 || len(discovered.Issues) != 1 || !strings.Contains(discovered.Issues[0].Error(), "timeout") {
		t.Fatalf("discovery = %+v", discovered)
	}
}

func TestExecutableCommandUsesBoundedJSONProtocol(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific")
	}
	root := t.TempDir()
	declared := validManifest("test.runner")
	script := "#!/bin/sh\nread request\nprintf '{\"protocol\":1,\"message\":\"hello from process\"}'\n"
	writePlugin(t, root, "runner", declared, script)
	commands, closeKernel := loadFixtureCommands(t, root)
	defer closeKernel()
	if len(commands) != 1 || commands[0].Execute == nil {
		t.Fatalf("commands = %+v", commands)
	}
	response, err := commands[0].Execute(t.Context(), terminal.CommandRequest{
		Argument: "world", Workspace: "/tmp/work", SessionID: "session-1",
	})
	if err != nil || response.Message != "hello from process" {
		t.Fatalf("response = %+v, %v", response, err)
	}
}

func TestDiscoveredCommandRejectsAReplacedExecutable(t *testing.T) {
	root := t.TempDir()
	declared := validManifest("test.replaced")
	writePlugin(t, root, "replaced", declared, "not executed")
	commands, closeKernel := loadFixtureCommands(t, root)
	defer closeKernel()
	if len(commands) != 1 || commands[0].Execute == nil {
		t.Fatalf("commands = %+v", commands)
	}
	entry := filepath.Join(root, "replaced", declared.Entry)
	replacement := entry + ".replacement"
	writeExecutable(t, replacement, "replacement must not execute")
	if err := os.Rename(replacement, entry); err != nil {
		t.Fatal(err)
	}
	if _, err := commands[0].Execute(t.Context(), terminal.CommandRequest{}); err == nil || !strings.Contains(err.Error(), "entry changed since discovery") {
		t.Fatalf("replaced executable error = %v", err)
	}
}

func loadFixtureCommands(t *testing.T, root string) ([]terminal.SlashCommand, func()) {
	t.Helper()
	discovered, err := New([]string{root}).Discover(t.Context())
	if err != nil || len(discovered.Issues) != 0 {
		t.Fatalf("discovery = %+v, %v", discovered, err)
	}
	registry := new(extensions.Registry)
	extensionHost, err := extensions.NewHost(registry)
	if err != nil {
		t.Fatal(err)
	}
	results, err := extensionHost.Activate(discovered.Plugins)
	if err != nil || len(results) != 1 || results[0].Phase != extensions.PluginLoaded {
		_ = extensionHost.Close()
		t.Fatalf("activation = %+v, %v", results, err)
	}
	return registry.Values(terminal.SlashCommands), func() { _ = extensionHost.Close() }
}

func TestExecutableCommandHonorsCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific")
	}
	root := t.TempDir()
	slow := filepath.Join(root, "slow")
	writeExecutable(t, slow, "#!/bin/sh\nsleep 2\n")
	identity, err := os.Stat(slow)
	if err != nil {
		t.Fatal(err)
	}
	executor := executableCommand{
		pluginID: "test.slow", command: "slow", source: executableSource{path: slow, identity: identity},
		directory: root, timeout: 20 * time.Millisecond,
	}
	_, err = executor.Execute(t.Context(), terminal.CommandRequest{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestCommandResponseRejectsMalformedProtocolWithoutProcessTiming(t *testing.T) {
	if _, err := decodeCommandResponse("test.bad", "bad", []byte("not json")); err == nil || !strings.Contains(err.Error(), "decode plugin") {
		t.Fatalf("malformed response error = %v", err)
	}
}

func TestCommandResponseNamesGenericTrailingJSON(t *testing.T) {
	_, err := decodeCommandResponse("test.bad", "bad", []byte(`{"protocol":1,"message":"ok"} {}`))
	if err == nil || !strings.Contains(err.Error(), "input contains multiple JSON values") || strings.Contains(err.Error(), "manifest") {
		t.Fatalf("trailing response error = %v", err)
	}
}

func TestCommandRequestIsBoundedBeforeAProcessStarts(t *testing.T) {
	executor := executableCommand{
		pluginID: "test.bounds", command: "large",
		source: executableSource{path: filepath.Join(t.TempDir(), "missing")}, timeout: time.Second,
	}
	_, err := executor.Execute(t.Context(), terminal.CommandRequest{Argument: strings.Repeat("x", maxCommandArgumentBytes+1)})
	if err == nil || !strings.Contains(err.Error(), "argument exceeds") || strings.Contains(err.Error(), "executable") {
		t.Fatalf("oversized request error = %v", err)
	}
}

func TestCommandEnvironmentUsesAnExplicitAllowlist(t *testing.T) {
	t.Setenv("FLAME_TEST_SECRET", "must-not-leak")
	t.Setenv("PATH", "/safe/bin")
	environment := commandEnvironment("test.safe", "hello")
	if !slices.Contains(environment, "PATH=/safe/bin") || !slices.Contains(environment, "FLAME_PLUGIN_ID=test.safe") || !slices.Contains(environment, "FLAME_PLUGIN_COMMAND=hello") {
		t.Fatalf("command environment = %v", environment)
	}
	if slices.ContainsFunc(environment, func(value string) bool { return strings.HasPrefix(value, "FLAME_TEST_SECRET=") }) {
		t.Fatalf("secret leaked into command environment: %v", environment)
	}
}
