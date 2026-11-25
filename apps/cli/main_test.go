package main

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/delivery/cmd"
	"github.com/Tangerg/flame/apps/cli/internal/delivery/terminal"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
)

func buildTestBinary(t *testing.T) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	return executable
}

func TestFlameProcess(t *testing.T) {
	if os.Getenv("FLAME_TEST_PROCESS") != "1" {
		return
	}
	flameHome, err := localruntime.ProductRoot()
	if err != nil {
		t.Fatal(err)
	}
	os.Args = append([]string{os.Args[0]}, flag.Args()...)
	runtime := runtimefixture.New()
	profile, err := runtimebinding.NewProfile(runtimefixture.CompleteDiscovery())
	if err != nil {
		t.Fatal(err)
	}
	announced := false
	announce := func() {
		if announced {
			return
		}
		_, _ = fmt.Fprintln(os.Stderr, testRuntimeReadyMarker)
		announced = true
	}
	stateDirectory := filepath.Join(flameHome, "cli")
	openWorkbench := workbenchFactory(stateDirectory)
	dependencies := cmd.Dependencies{
		OpenRuntime: func(context.Context, settings.Runtime) (cmd.Runtime, cmd.RuntimeProfile, error) {
			announce()
			return runtime, profile, nil
		},
		StartTerminal: func(ctx context.Context, request cmd.TerminalRequest) error {
			announce()
			configured := request.Settings.Clone()
			return terminal.Run(ctx, fixtureTerminalConfig(runtime, profile, terminal.Config{
				SessionID: request.SessionID, Workspace: request.Workspace,
				InitialPrompt: request.InitialPrompt, Settings: configured,
				OpenWorkbench: func() (*workbench.Store, error) {
					partition, err := request.Settings.Runtime.AuthoringPartition()
					if err != nil {
						return nil, err
					}
					return openWorkbench(partition)
				},
			}))
		},
		OpenWorkbench: openWorkbench,
	}
	os.Exit(runWithDependencies(dependencies, new(scriptedRuntimeOwner)))
}

func buildProductionBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "flame")
	if os.PathSeparator == '\\' {
		binary += ".exe"
	}
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build flame: %v\n%s", err, output)
	}
	directory, err := localruntime.FirstPartyDirectory(binary)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := localruntime.RunnerExecutable(binary)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimefixture.BuildRuntimeCompanions(t.Context(), binary, directory, runner); err != nil {
		t.Fatal(err)
	}
	return binary
}

func testProcessArguments(arguments ...string) []string {
	return append([]string{"-test.run=^TestFlameProcess$", "--", "--runtime-mode", "embedded"}, arguments...)
}

func TestProductionBinaryOpensAnIsolatedRuntimeAndReleasesItsLease(t *testing.T) {
	binary := buildProductionBinary(t)
	ambientHome := t.TempDir()
	t.Setenv("FLAME_HOME", ambientHome)
	environment := terminalTestEnvironment(t, map[string]string{
		"FLAME_PROVIDER": "anthropic", "FLAME_MODEL": "claude-opus-5", "FLAME_APIKEY": "test-key",
	})
	flameHome := isolatedProductHome(t, environment, ambientHome)

	profile := decodeCommandJSON[struct {
		Discovery protocol.DiscoverResponse `json:"discovery"`
	}](t, runTestBinary(t, binary, environment, "runtime", "info", "--json"), "in-process runtime profile")
	if profile.Discovery.ProtocolVersion == "" || profile.Discovery.ServerInfo.Name == "" || len(profile.Discovery.Capabilities.RuntimeTopics) == 0 {
		t.Fatalf("incomplete in-process runtime profile: %+v", profile)
	}
	data, err := localruntime.DataDirectoryUnder(flameHome)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(data.DatabasePath()); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("runtime database = (%v, %v), want regular file", info, err)
	}

	sessions := decodeCommandJSON[struct {
		Items []jsontext.Value `json:"items"`
	}](t, runTestBinary(t, binary, environment, "sessions", "ls", "--json"), "in-process session page")
	if len(sessions.Items) != 0 {
		t.Fatalf("fresh in-process runtime sessions = %d, want 0", len(sessions.Items))
	}
	entries, err := os.ReadDir(ambientHome)
	if err != nil || len(entries) != 0 {
		t.Fatalf("child process changed the ambient product home: %v, %v", entries, err)
	}
}

func isolatedProductHome(t *testing.T, environment []string, ambient string) string {
	t.Helper()
	for _, entry := range environment {
		if value, found := strings.CutPrefix(entry, "FLAME_HOME="); found {
			if value == "" || value == ambient {
				break
			}
			return value
		}
	}
	t.Fatal("test process did not acquire an isolated product home")
	return ""
}

func decodeCommandJSON[T any](t *testing.T, output []byte, what string) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(output, &value); err != nil {
		t.Fatalf("decode %s: %v\n%s", what, err, output)
	}
	return value
}

func runTestBinary(t *testing.T, binary string, environment []string, arguments ...string) []byte {
	t.Helper()
	command := exec.CommandContext(t.Context(), binary, append([]string{"--runtime-mode", "embedded"}, arguments...)...)
	command.Env = environment
	var diagnostics bytes.Buffer
	command.Stderr = &diagnostics
	output, err := command.Output()
	if err != nil {
		t.Fatalf("run %s: %v\n%s\n%s", strings.Join(arguments, " "), err, output, diagnostics.String())
	}
	if diagnostics.Len() > 0 {
		t.Logf("flame diagnostics:\n%s", diagnostics.String())
	}
	return output
}

func TestHeadlessBinaryReturnsConventionalSignalExitCodes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix signal exit semantics")
	}
	binary := buildTestBinary(t)
	for _, test := range []struct {
		name   string
		signal os.Signal
		want   int
	}{
		{name: "SIGINT", signal: os.Interrupt, want: 130},
		{name: "SIGTERM", signal: syscall.SIGTERM, want: 143},
	} {
		t.Run(test.name, func(t *testing.T) { requireHeadlessSignalExit(t, binary, test.signal, test.want) })
	}
}

func requireHeadlessSignalExit(t *testing.T, binary string, signal os.Signal, want int) {
	t.Helper()
	ready := make(chan struct{})
	diagnostics := &runtimeDiagnostics{ready: ready}
	command := exec.CommandContext(t.Context(), binary, testProcessArguments("run", "--json", "--approve-all", "wait for signal")...)
	command.Env = terminalTestEnvironment(t, nil)
	command.Stdout = io.Discard
	command.Stderr = diagnostics
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	waiting := true
	t.Cleanup(func() {
		if waiting {
			_ = command.Process.Kill()
			<-waited
		}
	})
	select {
	case <-ready:
	case err := <-waited:
		waiting = false
		t.Fatalf("process exited before opening the test runtime: %v\nstderr:\n%s", err, diagnostics.String())
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for headless run to start")
	}
	if err := command.Process.Signal(signal); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-waited:
		waiting = false
		exit, ok := errors.AsType[*exec.ExitError](err)
		if !ok || exit.ExitCode() != want {
			t.Fatalf("signal exit = %v, want code %d\nstderr:\n%s", err, want, diagnostics.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("headless run did not stop after signal")
	}
}

// Cmd owns the only writer and joins its copy loop before Wait returns, so the
// readiness observer cannot race process reclamation for an exposed pipe.
type runtimeDiagnostics struct {
	text  bytes.Buffer
	ready chan struct{}
}

func (d *runtimeDiagnostics) Write(data []byte) (int, error) {
	n, err := d.text.Write(data)
	if d.ready != nil && bytes.Contains(d.text.Bytes(), []byte(testRuntimeReadyMarker)) {
		close(d.ready)
		d.ready = nil
	}
	return n, err
}

func (d *runtimeDiagnostics) String() string { return d.text.String() }

const testRuntimeReadyMarker = "flame: test runtime ready"

type scriptedRuntimeOwner struct {
	closeErrors []error
	closes      int
}

func (s *scriptedRuntimeOwner) Close() error {
	index := s.closes
	s.closes++
	if index >= len(s.closeErrors) {
		return nil
	}
	return s.closeErrors[index]
}

func terminalTestEnvironment(t *testing.T, overrides map[string]string) []string {
	t.Helper()
	values := map[string]string{
		"COLORTERM": "", "LANG": "", "LC_ALL": "", "TERM_PROGRAM": "",
		"VSCODE_INJECTION": "", "WSL_INTEROP": "", "WSL_DISTRO_NAME": "",
		"FLAME_RUNTIME_CONFIG_DIR": "", "FLAME_TEST_PROCESS": "1",
		"FLAME_HOME": runtimefixture.TempDirectory(t),
	}
	maps.Copy(values, overrides)
	values["HOME"], values["USERPROFILE"] = t.TempDir(), t.TempDir()
	environment := make([]string, 0, len(os.Environ())+len(values))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := values[name]; !replaced {
			environment = append(environment, entry)
		}
	}
	for name, value := range values {
		environment = append(environment, name+"="+value)
	}
	return environment
}

// The fixture refuses every Runtime service beyond Sessions and Runs.
func fixtureTerminalConfig(runtime *runtimefixture.Runtime, profile terminal.RuntimeProfile, config terminal.Config) terminal.Config {
	config.Runtime, config.RuntimeProfile = runtime, profile
	config.Workspaces, config.Changes, config.Transfers = runtime, runtime, runtime
	config.Usage, config.ModelConfig, config.Goals, config.Skills = runtime, runtime, runtime, runtime
	config.MCP, config.Plugins, config.DiagnosticTools = runtime, runtime, runtimefixture.UnsupportedDiagnostics{}
	config.AuthoringContext, config.Hooks, config.Feedback = runtime, runtime, runtime
	return config
}
