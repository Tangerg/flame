package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Tangerg/flame/cli/internal/application/settings"
)

type flagBinding struct {
	key  string
	flag string
}

var settingFlagBindings = [...]flagBinding{
	{key: "provider", flag: "provider"},
	{key: "model", flag: "model"},
	{key: "ui.mouse", flag: "mouse"},
	{key: "ui.notifications", flag: "notifications"},
	{key: "ui.tool-details", flag: "tool-details"},
	{key: "ui.transcript-retain", flag: "transcript-retain"},
	{key: "ui.reconnect-attempts", flag: "reconnect-attempts"},
	{key: "plugins.directories", flag: "plugin-dir"},
}

type runLimitFlagKind uint8

const (
	runLimitTokens runLimitFlagKind = iota + 1
	runLimitSteps
	runLimitBudget
)

type runLimitFlagBinding struct {
	key, flag string
	kind      runLimitFlagKind
}

var runLimitFlagBindings = [...]runLimitFlagBinding{
	{key: "run.max-total-tokens", flag: "max-total-tokens", kind: runLimitTokens},
	{key: "run.max-steps", flag: "max-steps", kind: runLimitSteps},
	{key: "run.max-budget-usd", flag: "max-budget-usd", kind: runLimitBudget},
}

func configureRoot(v *viper.Viper, root *cobra.Command) {
	defaults := settings.Default()
	setDefaults(v, defaults)
	v.SetEnvPrefix("FLAME_CLI")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()

	flags := root.PersistentFlags()
	flags.String("config", "", "Configuration file (default: ./.flame.yaml or the user config directory)")
	flags.String("provider", defaults.Provider, "Optional provider override for new runs (must be paired with --model)")
	flags.String("model", defaults.Model, "Optional model override for new runs (must be paired with --provider)")
	flags.String("max-total-tokens", "", "Maximum cumulative tokens per run (strictly positive; omit for unlimited)")
	flags.String("max-steps", "", "Maximum steps per run (strictly positive; omit for unlimited)")
	flags.String("max-budget-usd", "", "Maximum run cost in USD (strictly positive; omit for unlimited)")
	flags.Bool("mouse", defaults.UI.Mouse, "Enable mouse input in the terminal UI")
	flags.Bool("notifications", defaults.UI.Notifications, "Enable terminal completion notifications")
	flags.Bool("tool-details", defaults.UI.ToolDetails, "Expand tool output and diffs by default")
	flags.Int("transcript-retain", defaults.UI.TranscriptRetain, "Finished blocks retained in the live terminal viewport")
	flags.Int("reconnect-attempts", defaults.UI.ReconnectAttempts, "Times to reconnect a dropped run subscription")
	flags.StringSlice("plugin-dir", defaults.Plugins.Directories, "Directory containing sideloaded plugins (repeatable)")
}

func setDefaults(v *viper.Viper, defaults settings.Config) {
	v.SetDefault("provider", defaults.Provider)
	v.SetDefault("model", defaults.Model)
	v.SetDefault("approval.remember", defaults.Approval.Remember)
	v.SetDefault("ui.mouse", defaults.UI.Mouse)
	v.SetDefault("ui.notifications", defaults.UI.Notifications)
	v.SetDefault("ui.tool-details", defaults.UI.ToolDetails)
	v.SetDefault("ui.transcript-retain", defaults.UI.TranscriptRetain)
	v.SetDefault("ui.reconnect-attempts", defaults.UI.ReconnectAttempts)
	v.SetDefault("plugins.directories", defaults.Plugins.Directories)
	for _, action := range slices.Sorted(maps.Keys(defaults.Keys)) {
		bindings := defaults.Keys[action]
		v.SetDefault("keys."+action, bindings)
	}
}

func loadConfig(v *viper.Viper, cmd *cobra.Command) error {
	path, err := cmd.Flags().GetString("config")
	if err != nil {
		return err
	}
	if selectConfigSourceErr := selectConfigSource(v, path); selectConfigSourceErr != nil {
		return selectConfigSourceErr
	}
	if readInConfigErr := v.ReadInConfig(); readInConfigErr != nil {
		_, notFound := errors.AsType[viper.ConfigFileNotFoundError](readInConfigErr)
		if path != "" || !notFound {
			return fmt.Errorf("read configuration: %w", readInConfigErr)
		}
	}
	if bindSettingFlagsErr := bindSettingFlags(v, cmd); bindSettingFlagsErr != nil {
		return bindSettingFlagsErr
	}
	if applyRunLimitFlagsErr := applyRunLimitFlags(v, cmd); applyRunLimitFlagsErr != nil {
		return applyRunLimitFlagsErr
	}
	_, err = readSettings(v)
	return err
}

func applyRunLimitFlags(v *viper.Viper, cmd *cobra.Command) error {
	for _, binding := range runLimitFlagBindings {
		flag := cmd.Root().PersistentFlags().Lookup(binding.flag)
		if flag == nil || !flag.Changed {
			continue
		}
		raw, err := cmd.Flags().GetString(binding.flag)
		if err != nil {
			return fmt.Errorf("read --%s: %w", binding.flag, err)
		}
		switch binding.kind {
		case runLimitTokens:
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value <= 0 {
				return fmt.Errorf("--%s must be a positive integer", binding.flag)
			}
			v.Set(binding.key, value)
		case runLimitSteps:
			value, err := strconv.ParseInt(raw, 10, strconv.IntSize)
			if err != nil || value <= 0 {
				return fmt.Errorf("--%s must be a positive integer", binding.flag)
			}
			v.Set(binding.key, int(value))
		case runLimitBudget:
			value, err := strconv.ParseFloat(raw, 64)
			if err != nil || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf("--%s must be a finite positive number", binding.flag)
			}
			v.Set(binding.key, value)
		default:
			return fmt.Errorf("--%s has an unknown run-limit type", binding.flag)
		}
	}
	return nil
}

func selectConfigSource(v *viper.Viper, explicitPath string) error {
	if explicitPath != "" {
		v.SetConfigFile(explicitPath)
		return nil
	}
	const projectConfig = ".flame.yaml"
	_, err := os.Stat(projectConfig)
	switch {
	case err == nil:
		v.SetConfigFile(projectConfig)
		return nil
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("inspect project configuration: %w", err)
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("resolve user config directory: %w", err)
	}
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(filepath.Join(configDir, "flame"))
	return nil
}

func bindSettingFlags(v *viper.Viper, cmd *cobra.Command) error {
	for _, binding := range settingFlagBindings {
		// A subcommand may own a same-named local flag with different semantics
		// (sessions update --model is a provider/model patch). Only the root's
		// persistent preference flags may feed CLI settings.
		flag := cmd.Root().PersistentFlags().Lookup(binding.flag)
		if flag == nil {
			continue
		}
		if err := v.BindPFlag(binding.key, flag); err != nil {
			return fmt.Errorf("bind --%s: %w", binding.flag, err)
		}
	}
	return nil
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
		config.Plugins.Directories = []string{filepath.Join(home, ".flame", "plugins")}
	}
	if err := config.Validate(); err != nil {
		return settings.Config{}, fmt.Errorf("validate configuration: %w", err)
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
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(config)
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
