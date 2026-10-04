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

	"github.com/Tangerg/flame/cli/internal/application/settings"
	"github.com/Tangerg/flame/cli/internal/application/workbench"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// version is overridden at link time via -ldflags "-X ...cmd.version=...".
var version = "dev"

// Version returns the client build identity advertised to runtime discovery.
func Version() string { return version }

const configIndependentAnnotation = "flame/config-independent"

// Dependencies are the outer implementations available to the command tree.
// Runtime construction stays lazy so help and completion-script generation do
// not open sockets, databases, or other process-owned resources. Dynamic value
// completion may resolve the Runtime when it needs authoritative catalog data.
type Dependencies struct {
	OpenRuntime   func(context.Context, string) (Runtime, RuntimeProfile, error)
	StartTerminal func(context.Context, TerminalRequest) error
	OpenWorkbench func(string) (*workbench.Store, error)
}

// TerminalRequest is the command-owned input needed to start one interactive
// delivery. Concrete Runtime adapters and terminal dependencies stay in main.
type TerminalRequest struct {
	SessionID      string
	Workspace      string
	LocalDirectory string
	InitialPrompt  string
	Settings       settings.Config
}

// runtimeProvider delays construction until a command needs the runtime. It
// owns delivery-only diagnostics so factories remain independent of Cobra.
type runtimeProvider struct {
	open          func(context.Context) (Runtime, RuntimeProfile, error)
	openWorkbench func(string) (*workbench.Store, error)
	configuration *viper.Viper
	prepare       func(*cobra.Command) error
}

func (r runtimeProvider) Runtime(cmd *cobra.Command) (Runtime, error) {
	runtime, _, err := r.Open(cmd)
	return runtime, err
}

func (r runtimeProvider) Open(cmd *cobra.Command) (Runtime, RuntimeProfile, error) {
	ctx := cmd.Context()
	if r.prepare != nil {
		if err := r.prepare(cmd); err != nil {
			return nil, nil, err
		}
	}
	if r.open == nil {
		return nil, nil, errors.New("runtime factory is required")
	}
	runtime, profile, err := r.open(ctx)
	if err != nil {
		return nil, nil, err
	}
	if runtime == nil {
		return nil, nil, errors.New("runtime factory returned no agent runtime")
	}
	if profile != nil {
		if err := profile.Validate(); err != nil {
			return nil, nil, err
		}
	}
	return runtime, profile, nil
}

// NewRoot builds an isolated command tree from process-owned dependencies.
func NewRoot(dependencies Dependencies) *cobra.Command {
	v := viper.New()
	loaded := false
	prepare := func(command *cobra.Command) error {
		if loaded {
			return nil
		}
		if err := loadConfig(v, command); err != nil {
			return err
		}
		loaded = true
		return nil
	}
	provider := runtimeProvider{configuration: v, prepare: prepare, openWorkbench: dependencies.OpenWorkbench}
	if dependencies.OpenRuntime != nil {
		provider.open = func(ctx context.Context) (Runtime, RuntimeProfile, error) {
			configured, err := readSettings(v)
			if err != nil {
				return nil, nil, err
			}
			return dependencies.OpenRuntime(ctx, configured.Runtime.Endpoint)
		}
	}
	root := newRootCommand(v, dependencies.StartTerminal, prepare)
	configureRoot(v, root)
	root.Flags().StringP("session", "s", "", "Open an existing session instead of a new one")
	root.PersistentFlags().StringP("cwd", "C", "", "Local directory for CLI configuration and attachments; also the embedded Runtime workspace")
	root.Flags().String("workspace", "", "Workspace path on the selected Runtime (default: local directory when embedded, Runtime default when remote)")
	root.MarkFlagsMutuallyExclusive("session", "workspace")
	root.AddGroup(
		&cobra.Group{ID: "work", Title: "Work:"},
		&cobra.Group{ID: "manage", Title: "Manage:"},
		&cobra.Group{ID: "setup", Title: "Setup:"},
	)
	addRootCommands(root, provider, v)
	return root
}

func newRootCommand(
	v *viper.Viper,
	startTerminal func(context.Context, TerminalRequest) error,
	prepare func(*cobra.Command) error,
) *cobra.Command {
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
			if configIndependent(cmd) {
				return nil
			}
			return prepare(cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := readSettings(v)
			if err != nil {
				return err
			}
			return runInteractive(cmd, args, startTerminal, config)
		},
	}
}

func configIndependent(cmd *cobra.Command) bool {
	return cmd.Annotations[configIndependentAnnotation] == "true" || cmd.Name() == cobra.ShellCompRequestCmd
}

func addRootCommands(root *cobra.Command, provider runtimeProvider, v *viper.Viper) {
	run := newRunCommand(provider, v)
	run.GroupID = "work"
	sessions := newSessionsCommand(provider)
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
	plugins := newPluginsCommand(provider)
	plugins.GroupID = "manage"
	root.AddCommand(run, sessions, runs, approvals, plugins, runtimeCommand, config, completion)
}

// runInteractive opens the terminal interface, seeding the field with whatever was typed
// on the command line.
//
// With no terminal to take over it says so and points at the command that does not
// need one, rather than failing with something about file descriptors: a program whose
// output is being piped wants text, not frames.
func runInteractive(
	cmd *cobra.Command,
	args []string,
	startTerminal func(context.Context, TerminalRequest) error,
	config settings.Config,
) error {
	if startTerminal == nil {
		return errors.New("terminal starter is required")
	}
	workspacePath, err := resolveWorkspace(cmd, config)
	if err != nil {
		return err
	}
	localDirectory, err := resolveLocalDirectory(cmd)
	if err != nil {
		return err
	}
	// Named for the flag rather than for the package it is handed to, so the package
	// stays reachable by its own name.
	sessionID, _ := cmd.Flags().GetString("session")

	err = startTerminal(cmd.Context(), TerminalRequest{
		SessionID:      sessionID,
		Workspace:      workspacePath,
		LocalDirectory: localDirectory,
		InitialPrompt:  strings.TrimSpace(strings.Join(args, " ")),
		Settings:       config.Clone(),
	})
	var unavailable interface {
		error
		TerminalUnavailable()
	}
	if errors.As(err, &unavailable) {
		return errors.New("no terminal to draw on; use `flame run` for a one-shot run")
	}
	return err
}

// resolveWorkspace leaves remote filesystem interpretation with Runtime.
func resolveWorkspace(cmd *cobra.Command, config settings.Config) (string, error) {
	workspace, _ := cmd.Flags().GetString("workspace")
	if config.Runtime.Endpoint != "" {
		return workspace, nil
	}
	if workspace != "" {
		return canonicalWorkspacePath(workspace)
	}
	return resolveLocalDirectory(cmd)
}

func resolveLocalDirectory(cmd *cobra.Command) (string, error) {
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

func (r runtimeProvider) workspacePath(path string) (string, error) {
	configured, err := readSettings(r.configuration)
	if err != nil {
		return "", err
	}
	if configured.Runtime.Endpoint != "" {
		return path, nil
	}
	return canonicalWorkspacePath(path)
}

func (r runtimeProvider) workbench() (*workbench.Store, error) {
	if r.openWorkbench == nil {
		return nil, errors.New("workbench factory is required")
	}
	configured, err := readSettings(r.configuration)
	if err != nil {
		return nil, err
	}
	authoring, err := r.openWorkbench(configured.Runtime.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("open CLI workbench: %w", err)
	}
	if authoring == nil {
		return nil, errors.New("workbench factory returned no authoring store")
	}
	return authoring, nil
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
