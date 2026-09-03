// Package config loads Flame's runtime settings via Viper.
//
// Sources, from lowest to highest precedence:
//
//  1. Built-in defaults
//  2. The first config.yaml found in caller-supplied search-directory order
//  3. Environment variables (FLAME_*)
//
// The yaml file is where the API key lives in dev; it is gitignored.
// Copy config/config.example.yaml → config/config.yaml and fill it in.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
	"github.com/spf13/viper"
)

const maximumRuntimeConfigBytes int64 = 256 << 10

// Load resolves configuration from yaml + env + defaults. A missing config
// file is fine (defaults + env only). Provider catalog validation, default
// model selection, and provider-specific API-key fallback are deliberately
// outside config-source parsing because they depend on the live provider
// catalog.
func Load(configDirectories []string) (Settings, error) {
	v := viper.New()
	v.SetConfigType("yaml")

	// No default provider — it must be set explicitly in config/config.yaml
	// or via FLAME_PROVIDER. (No vendor is privileged as the implicit default.)
	v.SetDefault("server.listen", "127.0.0.1:17171")
	v.SetDefault("server.noLocalToken", false)
	// Tool-result eviction is on by default. Enablement and threshold are
	// independent so zero never acts as a hidden feature flag.
	v.SetDefault("toolResultOffload.enabled", true)
	v.SetDefault("toolResultOffload.threshold", DefaultToolResultOffloadThreshold)

	// FLAME_* env override yaml (e.g. FLAME_PROVIDER, FLAME_SERVER_LISTEN).
	v.SetEnvPrefix(environmentPrefix.String())
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	configPath, content, err := readConfigFile(configDirectories)
	if err != nil {
		return Settings{}, err
	}
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadConfig(bytes.NewReader(content)); err != nil {
			return Settings{}, fmt.Errorf("config: read config file: %w", err)
		}
	}
	if err := validateConfigShape(v); err != nil {
		return Settings{}, err
	}

	provider := v.GetString("provider")
	if provider == "" {
		return Settings{}, errors.New("config: provider is required — set `provider:` in config/config.yaml or FLAME_PROVIDER (see providers.list for the supported set)")
	}

	model := v.GetString("model")

	// Viper applies FLAME_APIKEY over yaml `apiKey`; recover that source here so
	// later composition cannot mistake a process-scoped secret for durable input.
	apiKey := FileAPIKey(v.GetString("apiKey"))
	if environmentKey := apiKeyEnvironment.Value(); environmentKey != "" {
		apiKey = EnvironmentAPIKey(environmentKey)
	}

	servers, err := parseMCPServers(mcpServersEnvironment.Value())
	if err != nil {
		return Settings{}, fmt.Errorf("config: %s: %w", mcpServersEnvironment, err)
	}

	a2aAgents, err := parseA2AAgents(a2aAgentsEnvironment.Value())
	if err != nil {
		return Settings{}, fmt.Errorf("config: %s: %w", a2aAgentsEnvironment, err)
	}
	a2aAgents, err = addA2ARPCOrigins(a2aAgents, a2aOriginsEnvironment.Value())
	if err != nil {
		return Settings{}, fmt.Errorf("config: %s: %w", a2aOriginsEnvironment, err)
	}

	lspServers, err := loadLSPServers(v)
	if err != nil {
		return Settings{}, err
	}
	toolResultOffloadThreshold := v.GetInt("toolResultOffload.threshold")
	if toolResultOffloadThreshold <= 0 {
		return Settings{}, errors.New("config: toolResultOffload.threshold must be positive; use toolResultOffload.enabled: false to disable eviction")
	}

	return Settings{
		Provider:     provider,
		Model:        model,
		APIKey:       apiKey,
		BaseURL:      v.GetString("baseURL"),
		UtilityModel: v.GetString("utilityModel"),
		Online:       loadOnline(v),
		MCPServers:   servers,
		A2AAgents:    a2aAgents,
		LSPServers:   lspServers,

		ToolResultOffload: ToolResultOffloadSettings{
			Enabled:   v.GetBool("toolResultOffload.enabled"),
			Threshold: toolResultOffloadThreshold,
		},

		SandboxShell:         v.GetBool("sandbox.shell"),
		SandboxReadOnlyPaths: v.GetStringSlice("sandbox.readOnlyPaths"),

		Server: Server{
			Listen:         v.GetString("server.listen"),
			NoLocalToken:   v.GetBool("server.noLocalToken"),
			LocalTokenPath: v.GetString("server.localTokenPath"),
			CORSOrigins:    v.GetStringSlice("server.corsOrigins"),
		},
	}, nil
}

func readConfigFile(configDirectories []string) (string, []byte, error) {
	paths := make([]string, len(configDirectories))
	for index, directory := range configDirectories {
		if !filepath.IsAbs(directory) {
			return "", nil, fmt.Errorf("config: search directory %q must be absolute", directory)
		}
		paths[index] = filepath.Join(filepath.Clean(directory), "config.yaml")
	}
	for _, path := range paths {
		expected, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", nil, fmt.Errorf("config: inspect config file %q: %w", path, err)
		}
		file, opened, err := fileinput.OpenExpected(path, expected, maximumRuntimeConfigBytes)
		if err != nil {
			return "", nil, fmt.Errorf("config: open config file %q: %w", path, err)
		}
		content, readErr := io.ReadAll(io.LimitReader(file, maximumRuntimeConfigBytes+1))
		verifyErr := fileinput.VerifyPathVersion(file, opened, path)
		closeErr := file.Close()
		if readErr != nil {
			return "", nil, fmt.Errorf("config: read config file %q: %w", path, readErr)
		}
		if verifyErr != nil {
			return "", nil, fmt.Errorf("config: verify config file %q after reading: %w", path, verifyErr)
		}
		if closeErr != nil {
			return "", nil, fmt.Errorf("config: close config file %q: %w", path, closeErr)
		}
		if int64(len(content)) > maximumRuntimeConfigBytes {
			return "", nil, fmt.Errorf(
				"config: read config file %q: %w",
				path,
				fileinput.ErrTooLarge,
			)
		}
		return path, content, nil
	}
	return "", nil, nil
}

// configShape is the closed YAML vocabulary accepted at the process boundary.
// Settings is intentionally not reused here: it is the resolved application
// value, while sandbox and LSP retain their source nesting only in YAML.
type configShape struct {
	Provider          string                    `mapstructure:"provider"`
	Model             string                    `mapstructure:"model"`
	APIKey            string                    `mapstructure:"apiKey"`
	BaseURL           string                    `mapstructure:"baseURL"`
	UtilityModel      string                    `mapstructure:"utilityModel"`
	ToolResultOffload ToolResultOffloadSettings `mapstructure:"toolResultOffload"`
	Sandbox           struct {
		Shell         bool     `mapstructure:"shell"`
		ReadOnlyPaths []string `mapstructure:"readOnlyPaths"`
	} `mapstructure:"sandbox"`
	Server Server `mapstructure:"server"`
	Online Online `mapstructure:"online"`
	LSP    struct {
		Servers []LSPServer `mapstructure:"servers"`
	} `mapstructure:"lsp"`
}

func validateConfigShape(v *viper.Viper) error {
	var shape configShape
	if err := v.UnmarshalExact(&shape); err != nil {
		return fmt.Errorf("config: decode configuration: %w", err)
	}
	return nil
}
