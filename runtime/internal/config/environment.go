package config

import "os"

type environmentVariable string

const (
	environmentPrefix environmentVariable = "FLAME"
	apiKeyEnvironment environmentVariable = "FLAME_APIKEY"

	mcpServersEnvironment   environmentVariable = "FLAME_MCP_SERVERS"
	a2aAgentsEnvironment    environmentVariable = "FLAME_A2A_AGENTS"
	a2aOriginsEnvironment   environmentVariable = "FLAME_A2A_RPC_ORIGINS"
	jinaAPIKeyEnvironment   environmentVariable = "FLAME_JINA_API_KEY"
	tavilyAPIKeyEnvironment environmentVariable = "FLAME_TAVILY_API_KEY"
	httpHostsEnvironment    environmentVariable = "FLAME_HTTP_ALLOWED_HOSTS"

	mcpTokenEnvironmentPrefix = "FLAME_MCP_"
	mcpTokenEnvironmentSuffix = "_TOKEN"
)

func (e environmentVariable) String() string { return string(e) }

func (e environmentVariable) Value() string { return os.Getenv(e.String()) }

func mcpTokenEnvironment(name string) environmentVariable {
	return environmentVariable(mcpTokenEnvironmentPrefix + envTokenKey(name) + mcpTokenEnvironmentSuffix)
}
