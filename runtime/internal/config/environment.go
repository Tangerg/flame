package config

import "os"

type environmentVariable string

const (
	environmentPrefix environmentVariable = "FLAME"
	apiKeyEnvironment environmentVariable = "FLAME_APIKEY"

	a2aAgentsEnvironment  environmentVariable = "FLAME_A2A_AGENTS"
	a2aOriginsEnvironment environmentVariable = "FLAME_A2A_RPC_ORIGINS"
)

func (e environmentVariable) String() string { return string(e) }

func (e environmentVariable) Value() string { return os.Getenv(e.String()) }
