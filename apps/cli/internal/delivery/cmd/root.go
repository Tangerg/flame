package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const developmentVersion = "dev"

var version = developmentVersion

func Version() string {
	switch {
	case version == "" || version == developmentVersion:
		return developmentVersion
	case strings.HasPrefix(version, "v"):
		return version
	default:
		return "v" + version
	}
}

const (
	configIndependentAnnotation = "flame/config-independent"
	annotationSet               = "true"
)

type Dependencies struct {
	OpenRuntime   func(context.Context, settings.Runtime) (Runtime, RuntimeProfile, error)
	StartTerminal func(context.Context, TerminalRequest) error
	OpenWorkbench func(string) (*workbench.Store, error)
}

type TerminalRequest struct {
	SessionID      string
	Workspace      string
	LocalDirectory string
	InitialPrompt  string
	Settings       settings.Config
}

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
	if err := r.prepare(cmd); err != nil {
		return nil, nil, err
	}
	runtime, profile, err := r.open(ctx)
	if err != nil {
		return nil, nil, err
	}
	if runtime == nil {
		return nil, nil, errors.New("runtime factory returned no agent runtime")
	}
	if profile == nil {
		return nil, nil, errors.New("runtime factory returned no negotiated profile")
	}
	return runtime, profile, nil
}

const (
	groupWork   = "work"
	groupManage = "manage"
	groupSetup  = "setup"
)

func NewRoot(dependencies Dependencies) (*cobra.Command, error) {
	if dependencies.OpenRuntime == nil || dependencies.StartTerminal == nil || dependencies.OpenWorkbench == nil {
		return nil, errors.New("cli: runtime, terminal and workbench factories are required")
	}
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
	provider := runtimeProvider{configuration: v, prepare: prepare, openWorkbench: dependencies.OpenWorkbench,
		open: func(ctx context.Context) (Runtime, RuntimeProfile, error) {
			configured, err := readSettings(v)
			if err != nil {
				return nil, nil, err
			}
			return dependencies.OpenRuntime(ctx, configured.Runtime)
		},
	}
	root := newRootCommand(v, dependencies.StartTerminal, prepare)
	configureRoot(v, root)
	root.Flags().StringP(sessionFlag, "s", "", "Open an existing session instead of a new one")
	root.PersistentFlags().StringP(cwdFlag, "C", "", "Local directory for CLI configuration and attachments; also the embedded Runtime workspace")
	root.Flags().String(workspaceFlag, "", "Workspace path on the selected Runtime (default: local directory when embedded, Runtime default when remote)")
	root.MarkFlagsMutuallyExclusive(sessionFlag, workspaceFlag)
	root.AddGroup(
		&cobra.Group{ID: groupWork, Title: "Work:"},
		&cobra.Group{ID: groupManage, Title: "Manage:"},
		&cobra.Group{ID: groupSetup, Title: "Setup:"},
	)
	addRootCommands(root, provider, v)
	return root, nil
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
		Version:       Version(),
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
	return cmd.Annotations[configIndependentAnnotation] == annotationSet || cmd.Name() == cobra.ShellCompRequestCmd
}

func addRootCommands(root *cobra.Command, provider runtimeProvider, v *viper.Viper) {
	run := newRunCommand(provider, v)
	run.GroupID = groupWork
	sessions := newSessionsCommand(provider)
	sessions.GroupID = groupManage
	runs := newRunsCommand(provider)
	runs.GroupID = groupManage
	approvals := newApprovalsCommand(provider)
	approvals.GroupID = groupManage
	runtimeCommand := newRuntimeCommand(provider)
	runtimeCommand.GroupID = groupManage
	config := newConfigCommand(v)
	config.GroupID = groupSetup
	completion := newCompletionCommand(root)
	completion.GroupID = groupSetup
	plugins := newPluginsCommand(provider)
	plugins.GroupID = groupManage
	root.AddCommand(run, sessions, runs, approvals, plugins, runtimeCommand, config, completion)
}

func runInteractive(
	cmd *cobra.Command,
	args []string,
	startTerminal func(context.Context, TerminalRequest) error,
	config settings.Config,
) error {
	workspacePath, err := resolveWorkspace(cmd, config)
	if err != nil {
		return err
	}
	localDirectory, err := resolveLocalDirectory(cmd)
	if err != nil {
		return err
	}
	sessionID, _ := cmd.Flags().GetString(sessionFlag)

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

func resolveWorkspace(cmd *cobra.Command, config settings.Config) (string, error) {
	workspace, _ := cmd.Flags().GetString(workspaceFlag)
	if config.Runtime.Mode == settings.RuntimeRemote {
		return workspace, nil
	}
	if workspace != "" {
		return canonicalWorkspacePath(workspace)
	}
	return resolveLocalDirectory(cmd)
}

func resolveLocalDirectory(cmd *cobra.Command) (string, error) {
	cwd, _ := cmd.Flags().GetString(cwdFlag)
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
	if configured.Runtime.Mode == settings.RuntimeRemote {
		return path, nil
	}
	return canonicalWorkspacePath(path)
}

func (r runtimeProvider) workbench() (*workbench.Store, error) {
	configured, err := readSettings(r.configuration)
	if err != nil {
		return nil, err
	}
	partition, err := configured.Runtime.AuthoringPartition()
	if err != nil {
		return nil, err
	}
	authoring, err := r.openWorkbench(partition)
	if err != nil {
		return nil, fmt.Errorf("open CLI workbench: %w", err)
	}
	if authoring == nil {
		return nil, errors.New("workbench factory returned no authoring store")
	}
	return authoring, nil
}

func canonicalWorkspacePath(path string) (string, error) {
	root := ""
	if !filepath.IsAbs(path) {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve workspace: %w", err)
		}
	}
	return filesystem.ResolvePath(root, path)
}

var errNoPrompt = errors.New("no prompt: pass one as an argument, pipe one in, or both")
