// Package bootstrap is the composition root: it adapts process config and
// environment into runtime construction inputs, wires the rings, and owns host
// lifecycle.
package bootstrap

import (
	"errors"
	"fmt"
	"os"

	"github.com/Tangerg/flame/runtime/internal/adapter/providerregistry"
	"github.com/Tangerg/flame/runtime/internal/application/models"
	"github.com/Tangerg/flame/runtime/internal/config"
	mcpserversvc "github.com/Tangerg/flame/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/infra/llm"
)

// LoadConfig loads the app config and resolves provider defaults plus env-key
// overrides used by the runtime process.
func LoadConfig(configDirectories []string) (config.Settings, error) {
	cfg, err := config.Load(configDirectories)
	if err != nil {
		return config.Settings{}, err
	}
	return resolveProviderConfig(cfg)
}

func resolveProviderConfig(settings config.Settings) (config.Settings, error) {
	profile, found := llm.LookupProvider(llm.Provider(settings.Provider))
	if !found {
		return config.Settings{}, fmt.Errorf("config: unknown provider %q (see providers.list for the supported set)", settings.Provider)
	}
	if settings.Model == "" {
		defaultModel, hasDefault := profile.DefaultChatModel()
		if !hasDefault {
			return config.Settings{}, fmt.Errorf("config: provider %q requires an explicit model", settings.Provider)
		}
		settings.Model = defaultModel
	}
	apiKeyEnvironmentVariable := profile.CredentialEnvironment()
	if apiKey := os.Getenv(apiKeyEnvironmentVariable); apiKey != "" {
		settings.APIKey = config.EnvironmentAPIKey(apiKey)
	}
	if profile.RequiresAPIKey() && !settings.APIKey.Present() {
		return config.Settings{}, errors.New("config: apiKey is empty — set it in config/config.yaml or " + apiKeyEnvironmentVariable)
	}
	return settings, nil
}

// ProviderRegistry wraps the durable provider registry with env-key fallback.
func ProviderRegistry(registry models.ProviderRegistry, settings config.Settings) (models.ProviderRegistry, error) {
	environmentKeys := llm.EnvKeys()
	if apiKey, present := settings.APIKey.EnvironmentValue(); present {
		environmentKeys[settings.Provider] = apiKey
	}
	return providerregistry.WithEnvironmentKeys(registry, environmentKeys)
}

// MCPServers projects config-file MCP entries into the runtime registry model.
// It rejects an unknown transport instead of preserving an invalid string for a
// later dial attempt; configuration is an input boundary, not a best-effort
// transport pass-through.
func MCPServers(configuredServers []config.MCPServer) ([]mcpserversvc.Server, error) {
	if len(configuredServers) == 0 {
		return nil, nil
	}
	servers := make([]mcpserversvc.Server, len(configuredServers))
	for index, server := range configuredServers {
		name, err := mcpserversvc.ParseServerName(server.Name)
		if err != nil {
			return nil, fmt.Errorf("config: MCP server %q: %w", server.Name, err)
		}
		transport, err := parseMCPTransport(server.Transport)
		if err != nil {
			return nil, fmt.Errorf("config: MCP server %q: %w", server.Name, err)
		}
		candidate := mcpserversvc.Server{
			Name:          name,
			Transport:     transport,
			Enabled:       true,
			URL:           server.Endpoint,
			Authorization: server.Authorization,
			Command:       server.Command,
			Args:          append([]string(nil), server.Args...),
		}
		if err := candidate.Validate(); err != nil {
			return nil, fmt.Errorf("config: MCP server %q: %w", server.Name, err)
		}
		servers[index] = candidate
	}
	return servers, nil
}

func parseMCPTransport(transport config.MCPTransport) (mcpserversvc.Transport, error) {
	if !transport.Valid() {
		return "", fmt.Errorf("unknown transport %q", transport)
	}
	if transport == config.MCPTransportStreamableHTTP {
		return mcpserversvc.TransportStreamableHTTP, nil
	}
	return mcpserversvc.TransportStdio, nil
}
