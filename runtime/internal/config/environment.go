package config

import "os"

type environmentVariable string

const (
	environmentPrefix environmentVariable = "FLAME"
	apiKeyEnvironment environmentVariable = "FLAME_APIKEY"
)

func (e environmentVariable) String() string { return string(e) }

func (e environmentVariable) Value() string { return os.Getenv(e.String()) }
