package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/keybinding"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

const maximumCLIConfigBytes int64 = 256 << 10

const (
	runtimeModeKey       = "runtime.mode"
	runtimeEndpointKey   = "runtime.endpoint"
	pluginDirectoriesKey = "plugins.directories"
)

func configureRoot(v *viper.Viper, root *cobra.Command) {
	defaults := settings.Default()
	v.SetEnvPrefix("FLAME_CLI")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()

	flags := root.PersistentFlags()
	flags.String(configFlag, "", "YAML configuration file (default: <workspace>/.flame.yaml or the user config directory)")
	preference(v, flags, flags.String, runtimeModeKey, "runtime-mode", string(defaults.Runtime.Mode), "Runtime connection mode: remote or embedded (exclusively owns local data)")
	preference(v, flags, flags.String, runtimeEndpointKey, "runtime-url", defaults.Runtime.Endpoint, "Existing Runtime base URL for remote mode")
	preference(v, flags, flags.String, "provider", providerFlag, defaults.Provider, "Optional provider override for new runs (must be paired with --model)")
	preference(v, flags, flags.String, "model", modelFlag, defaults.Model, "Optional model override for new runs (must be paired with --provider)")
	preference(v, flags, flags.Bool, "ui.mouse", "mouse", defaults.UI.Mouse, "Enable mouse input in the terminal UI")
	preference(v, flags, flags.Bool, "ui.notifications", "notifications", defaults.UI.Notifications, "Enable terminal completion notifications")
	preference(v, flags, flags.Bool, "ui.tool-details", "tool-details", defaults.UI.ToolDetails, "Expand tool output and diffs by default")
	preference(v, flags, flags.Int, "ui.transcript-retain", "transcript-retain", defaults.UI.TranscriptRetain, "Finished blocks retained in the live terminal viewport")
	preference(v, flags, flags.StringSlice, pluginDirectoriesKey, "plugin-dir", defaults.Plugins.Directories, "Directory containing sideloaded plugins (repeatable)")
	v.SetDefault("approval.remember", defaults.Approval.Remember)
	for _, action := range slices.Sorted(maps.Keys(defaults.Keys)) {
		v.SetDefault("keys."+string(action), defaults.Keys[action])
	}
}

// Binding the root's own persistent flag keeps a subcommand's same-named local flag (sessions update
// --model is a provider/model patch) from feeding CLI settings.
func preference[T any](v *viper.Viper, flags *pflag.FlagSet, define func(name string, value T, usage string) *T, key, flag string, value T, usage string) {
	define(flag, value, usage)
	v.SetDefault(key, value)
	if err := v.BindPFlag(key, flags.Lookup(flag)); err != nil {
		panic(err)
	}
}

type configAuthority uint8

const (
	projectConfigAuthority configAuthority = iota
	explicitConfigAuthority
	userConfigAuthority
)

type selectedConfigSource struct {
	path      string
	authority configAuthority
}

func loadConfig(v *viper.Viper, cmd *cobra.Command) error {
	path, err := cmd.Flags().GetString(configFlag)
	if err != nil {
		return err
	}
	source, err := selectConfigSource(cmd, path)
	if err != nil {
		return err
	}
	if source != nil {
		content, err := readConfigFile(source.path)
		if err != nil {
			return fmt.Errorf("read configuration %q: %w", source.path, err)
		}
		v.SetConfigType("yaml")
		v.SetConfigFile(source.path)
		if err := v.ReadConfig(bytes.NewReader(content)); err != nil {
			return fmt.Errorf("read configuration %q: %w", source.path, err)
		}
		// Project preferences cannot grant native code execution or route a process-owned bearer.
		// Validate the already-read bytes, not a second read of a mutable file.
		if source.authority == projectConfigAuthority && v.InConfig(pluginDirectoriesKey) {
			return errors.New("project configuration cannot select executable plugin directories; use --plugin-dir, user configuration, or an explicit --config")
		}
		if source.authority == projectConfigAuthority && (v.InConfig(runtimeEndpointKey) || v.InConfig(runtimeModeKey)) {
			return errors.New("project configuration cannot select a runtime endpoint; use --runtime-url, FLAME_CLI_RUNTIME_ENDPOINT, user configuration, or an explicit --config")
		}
	}
	_, err = readSettings(v)
	return err
}

func selectConfigSource(cmd *cobra.Command, explicitPath string) (*selectedConfigSource, error) {
	if explicitPath != "" {
		if !strings.EqualFold(filepath.Ext(explicitPath), ".yaml") {
			return nil, errors.New("--config must name a .yaml file")
		}
		return &selectedConfigSource{path: explicitPath, authority: explicitConfigAuthority}, nil
	}
	workspace, err := resolveLocalDirectory(cmd)
	if err != nil {
		return nil, fmt.Errorf("resolve project configuration workspace: %w", err)
	}
	projectConfig := filepath.Join(workspace, ".flame.yaml")
	_, err = os.Stat(projectConfig)
	switch {
	case err == nil:
		return &selectedConfigSource{path: projectConfig, authority: projectConfigAuthority}, nil
	case !errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("inspect project configuration: %w", err)
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user config directory: %w", err)
	}
	userConfig := filepath.Join(configDir, "flame", "config.yaml")
	_, err = os.Stat(userConfig)
	switch {
	case err == nil:
		return &selectedConfigSource{path: userConfig, authority: userConfigAuthority}, nil
	case errors.Is(err, os.ErrNotExist):
		return nil, nil
	default:
		return nil, fmt.Errorf("inspect user configuration: %w", err)
	}
}

func readConfigFile(path string) ([]byte, error) {
	file, opened, err := filesystem.OpenFile(path, maximumCLIConfigBytes)
	if err != nil {
		return nil, err
	}
	content, readErr := io.ReadAll(io.LimitReader(file, maximumCLIConfigBytes+1))
	verifyErr := filesystem.VerifyPathVersion(file, opened, path)
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if verifyErr != nil {
		return nil, verifyErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(content)) > maximumCLIConfigBytes {
		return nil, filesystem.FileSizeLimitError{
			Size: int64(len(content)), Limit: maximumCLIConfigBytes,
		}
	}
	return content, nil
}

func readSettings(v *viper.Viper) (settings.Config, error) {
	var config settings.Config
	if err := v.UnmarshalExact(&config); err != nil {
		return settings.Config{}, fmt.Errorf("decode configuration: %w", err)
	}
	if len(config.Plugins.Directories) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return settings.Config{}, fmt.Errorf("resolve home directory for plugins: %w", err)
		}
		config.Plugins.Directories = []string{settings.DefaultPluginDirectory(home)}
	}
	if err := config.Validate(); err != nil {
		return settings.Config{}, fmt.Errorf("validate configuration: %w", err)
	}
	if _, err := keybinding.Parse(config.Keys); err != nil {
		return settings.Config{}, err
	}
	return config.Clone(), nil
}

func newConfigCommand(v *viper.Viper) *cobra.Command {
	config := &cobra.Command{Use: "config", Short: "Inspect the effective configuration", Args: cobra.NoArgs}
	config.AddCommand(&cobra.Command{
		Use:          "show",
		Short:        "Print the merged, validated configuration as JSON",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			config, err := readSettings(v)
			if err != nil {
				return err
			}
			return writeIndentedJSON(cmd.OutOrStdout(), config)
		},
	})
	config.AddCommand(&cobra.Command{
		Use:          "path",
		Short:        "Print the configuration file that was loaded",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			used := v.ConfigFileUsed()
			if used == "" {
				used = "(defaults, environment, and flags only)"
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), used)
			return err
		},
	})
	return config
}
