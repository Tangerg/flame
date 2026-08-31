// Package cmd is the CLI's command tree.
//
// Commands are built by constructors rather than declared as package variables,
// so a test can build a fresh tree with its own runtime and its own output
// buffers. Flag state does not survive between trees, which is what makes the
// commands testable in-memory.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Tangerg/oolong/core/term"

	"github.com/Tangerg/flame/cli/internal/agent"
	"github.com/Tangerg/flame/cli/internal/backend"
	"github.com/Tangerg/flame/cli/internal/extensions"
	"github.com/Tangerg/flame/cli/internal/settings"
	"github.com/Tangerg/flame/cli/internal/sideload"
	terminalui "github.com/Tangerg/flame/cli/internal/terminal"
)

// version is overridden at link time via -ldflags "-X ...cmd.version=...".
var version = "dev"

// Version returns the client build identity advertised to runtime discovery.
func Version() string { return version }

const configIndependentAnnotation = "flame/config-independent"

// Dependencies are the outer implementations available to the command tree.
// Runtime construction stays lazy so help and completion do not open sockets,
// databases, or other process-owned resources.
type Dependencies struct {
	OpenRuntime    func(context.Context) (backend.Services, error)
	StateDirectory string
}

// runtimeProvider delays construction until a command needs the runtime. It
// owns delivery-only diagnostics so factories remain independent of Cobra.
type runtimeProvider struct {
	open func(context.Context) (backend.Services, error)
}

func (r runtimeProvider) Open(cmd *cobra.Command) (agent.Runtime, error) {
	services, err := r.OpenServices(cmd)
	if err != nil {
		return nil, err
	}
	return services.Agent, nil
}

func (r runtimeProvider) OpenServices(cmd *cobra.Command) (backend.Services, error) {
	services, err := r.resolve(cmd.Context())
	if err != nil {
		return backend.Services{}, err
	}
	return services, nil
}

func (r runtimeProvider) OpenQuietly(cmd *cobra.Command) (agent.Runtime, error) {
	services, err := r.resolve(cmd.Context())
	return services.Agent, err
}

func (r runtimeProvider) resolve(ctx context.Context) (backend.Services, error) {
	if r.open == nil {
		return backend.Services{}, errors.New("runtime factory is required")
	}
	services, err := r.open(ctx)
	if err != nil {
		return backend.Services{}, err
	}
	if err := services.Validate(); err != nil {
		return backend.Services{}, err
	}
	return services, nil
}

// NewRoot builds an isolated command tree from process-owned dependencies.
func NewRoot(dependencies Dependencies) *cobra.Command {
	provider := runtimeProvider{open: dependencies.OpenRuntime}
	v := viper.New()
	root := newRootCommand(v, provider, dependencies.StateDirectory)
	configureRoot(v, root)
	root.Flags().StringP("session", "s", "", "Open an existing session instead of a new one")
	root.PersistentFlags().StringP("cwd", "C", "", "Workspace directory for a new session (default: current directory)")
	root.AddGroup(
		&cobra.Group{ID: "work", Title: "Work:"},
		&cobra.Group{ID: "manage", Title: "Manage:"},
		&cobra.Group{ID: "setup", Title: "Setup:"},
	)
	addRootCommands(root, provider, v, dependencies.StateDirectory)
	return root
}

func newRootCommand(v *viper.Viper, provider runtimeProvider, stateDirectory string) *cobra.Command {
	return &cobra.Command{
		Use:   "flame [prompt...]",
		Short: "Terminal front end for the flame agent runtime",
		Long: "flame drives an agent runtime from the terminal: an interactive session by\n" +
			"default, and one-shot runs for scripts and pipelines.",
		Example: "  # Interactive\n" +
			"  flame\n\n" +
			"  # One-shot run, output written for a person\n" +
			"  flame run \"why is TestCacheExpiry flaky?\"\n\n" +
			"  # One-shot run, output written for a program\n" +
			"  flame run --json \"why is TestCacheExpiry flaky?\" > result.json\n\n" +
			"  # Stream every run event as newline-delimited JSON\n" +
			"  flame run --output-format streaming-json \"trace the flaky test\" > run.ndjson\n\n" +
			"  # Feed a file in as context\n" +
			"  cat cache_test.go | flame run \"explain what this test is really waiting for\"\n\n" +
			"  # List sessions\n" +
			"  flame sessions ls",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Annotations[configIndependentAnnotation] == "true" {
				return nil
			}
			return loadConfig(v, cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := readSettings(v)
			if err != nil {
				return err
			}
			return runInteractive(cmd, args, provider, config, stateDirectory)
		},
	}
}

func addRootCommands(root *cobra.Command, provider runtimeProvider, v *viper.Viper, stateDirectory string) {
	run := newRunCommand(provider, v)
	run.GroupID = "work"
	sessions := newSessionsCommand(provider, stateDirectory)
	sessions.GroupID = "manage"
	runs := newRunsCommand(provider)
	runs.GroupID = "manage"
	approvals := newApprovalsCommand(provider)
	approvals.GroupID = "manage"
	runtimeCommand := newRuntimeCommand(provider)
	runtimeCommand.GroupID = "manage"
	config := newConfigCommand(v)
	config.GroupID = "setup"
	completion := newCompletionCommand(root)
	completion.GroupID = "setup"
	root.AddCommand(run, sessions, runs, approvals, runtimeCommand, config, completion)
}

// runInteractive opens the terminal interface, seeding the field with whatever was typed
// on the command line.
//
// With no terminal to take over it says so and points at the command that does not
// need one, rather than failing with something about file descriptors: a program whose
// output is being piped wants text, not frames.
func runInteractive(cmd *cobra.Command, args []string, provider runtimeProvider, config settings.Config, stateDirectory string) error {
	services, err := provider.OpenServices(cmd)
	if err != nil {
		return err
	}
	workspacePath, err := resolveWorkspace(cmd)
	if err != nil {
		return err
	}
	// Named for the flag rather than for the package it is handed to, so the package
	// stays reachable by its own name.
	sessionID, _ := cmd.Flags().GetString("session")

	err = terminalui.Run(cmd.Context(), terminalui.Config{
		Services:       services,
		ClientVersion:  version,
		SessionID:      sessionID,
		Workspace:      workspacePath,
		InitialPrompt:  strings.TrimSpace(strings.Join(args, " ")),
		Settings:       new(config),
		PluginSources:  []extensions.Source{sideload.New(config.Plugins.Directories)},
		StateDirectory: stateDirectory,
	})
	if errors.Is(err, term.ErrNotTerminal) {
		return errors.New("no terminal to draw on; use `flame run` for a one-shot run")
	}
	return err
}

// resolveWorkspace resolves the directory a session works in.
func resolveWorkspace(cmd *cobra.Command) (string, error) {
	cwd, _ := cmd.Flags().GetString("cwd")
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory: %w", err)
		}
	}
	return canonicalWorkspacePath(cwd)
}

func canonicalWorkspacePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	abs = filepath.Clean(abs)
	canonical, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return canonical, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("resolve workspace symlinks: %w", err)
	}
	return abs, nil
}

var errNoPrompt = errors.New("no prompt: pass one as an argument, pipe one in, or both")
