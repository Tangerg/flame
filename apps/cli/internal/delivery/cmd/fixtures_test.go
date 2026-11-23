package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
)

func commandRuntimeProfile(t *testing.T) runtimebinding.Profile {
	t.Helper()
	profile, err := runtimebinding.NewProfile(runtimefixture.CompleteDiscovery())
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

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
		StartTerminal: func(context.Context, TerminalRequest) error {
			return errors.New("the terminal is not part of command tests")
		},
	}
	root, err := NewRoot(dependencies)
	if err != nil {
		t.Fatal(err)
	}
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
	err = root.ExecuteContext(t.Context())
	return out.String(), errb.String(), err
}
