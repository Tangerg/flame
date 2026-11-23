package config

import (
	"fmt"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/secret"
)

type apiKeySource uint8

const (
	apiKeyFromFile apiKeySource = iota + 1
	apiKeyFromEnvironment
)

type APIKeyInput struct {
	value  string
	source apiKeySource
}

func FileAPIKey(value string) APIKeyInput {
	if value == "" {
		return APIKeyInput{}
	}
	return APIKeyInput{value: value, source: apiKeyFromFile}
}

func EnvironmentAPIKey(value string) APIKeyInput {
	if value == "" {
		return APIKeyInput{}
	}
	return APIKeyInput{value: value, source: apiKeyFromEnvironment}
}

func (k APIKeyInput) Present() bool { return k.value != "" }

func (k APIKeyInput) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte(secret.Redacted))
}

func (k APIKeyInput) FileValue() (string, bool) {
	return k.value, k.Present() && k.source == apiKeyFromFile
}

func (k APIKeyInput) EnvironmentValue() (string, bool) {
	return k.value, k.Present() && k.source == apiKeyFromEnvironment
}

type Server struct {
	Listen         string
	NoLocalToken   bool
	LocalTokenPath string
	CORSOrigins    []string
	WebDirectory   string
}

type Settings struct {
	DefaultSelection flamemodel.Selection
	APIKey           APIKeyInput
	BaseURL          string
	UtilityModel     string

	ToolResultOffloadThreshold *int

	SandboxShell         bool
	SandboxReadOnlyPaths []string

	Server Server
}

const defaultToolResultOffloadThreshold = 50_000
