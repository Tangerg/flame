package cmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func commandRuntimeProfile(t *testing.T, edits ...func(*protocol.DiscoverResponse, *protocol.ClientCapabilities)) runtimefixture.Profile {
	t.Helper()
	discovery, client := runtimefixture.CompleteDiscovery()
	for _, edit := range edits {
		edit(discovery, client)
	}
	profile, err := runtimefixture.NewProfile(discovery, client)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

// executeCommand runs the CLI in memory and returns stdout, stderr and the command error.
// Nothing here touches the process's own streams, which is what lets the tests
// run in parallel and assert on exact output.
func executeCommand(t *testing.T, rt Runtime, stdin string, args ...string) (string, string, error) {
	t.Helper()
	if rt == nil {
		rt = runtimefixture.New()
	}
	return executeCommandWithRuntime(t, rt, new(commandRuntimeProfile(t)), stdin, args...)
}

func instantRuntime() *runtimefixture.Runtime {
	rt := runtimefixture.New()
	rt.Instant = true
	return rt
}

func executeCommandWithRuntime(
	t *testing.T,
	runtime Runtime,
	profile RuntimeProfile,
	stdin string,
	args ...string,
) (string, string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	dependencies := Dependencies{
		OpenRuntime:   func(context.Context, settings.Runtime) (Runtime, RuntimeProfile, error) { return runtime, profile, nil },
		OpenWorkbench: func(string) (*workbench.Store, error) { return workbench.OpenMemory(workbench.Config{}) },
	}
	root := NewRoot(dependencies)
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetIn(strings.NewReader(stdin))
	localTarget := true
	for _, arg := range args {
		if strings.HasPrefix(arg, "--runtime-") || arg == "--config" {
			localTarget = false
		}
	}
	if localTarget && os.Getenv("FLAME_CLI_RUNTIME_ENDPOINT") == "" {
		args = append([]string{"--runtime-mode", "embedded"}, args...)
	}
	root.SetArgs(args)
	err := root.ExecuteContext(t.Context())
	return out.String(), errb.String(), err
}
