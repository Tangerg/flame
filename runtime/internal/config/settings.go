package config

import (
	"fmt"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
)

type apiKeySource uint8

const (
	apiKeyFromFile apiKeySource = iota + 1
	apiKeyFromEnvironment
)

// APIKeyInput keeps credential material inseparable from the process source
// that supplied it. A file key may seed durable provider configuration; an
// environment key is process-scoped and must remain an in-memory overlay.
// The zero value is the explicit absence of a credential.
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

// Format makes every fmt verb a redaction boundary, including recursive %+v
// and %#v formatting of Settings. Credential material is only available via
// the two source-specific accessors below.
func (k APIKeyInput) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte("[REDACTED]"))
}

// FileValue exposes a credential only to the durable configuration boundary.
func (k APIKeyInput) FileValue() (string, bool) {
	return k.value, k.Present() && k.source == apiKeyFromFile
}

// EnvironmentValue exposes a credential only to the process overlay boundary.
func (k APIKeyInput) EnvironmentValue() (string, bool) {
	return k.value, k.Present() && k.source == apiKeyFromEnvironment
}

type Server struct {
	Listen         string
	NoLocalToken   bool
	LocalTokenPath string
	CORSOrigins    []string // empty → server falls back to the built-in dev allowlist
	WebDirectory   string   // optional absolute path to the built browser application
}

type Settings struct {
	// DefaultSelection is exact: a Run that names no model runs against it.
	DefaultSelection modelchoice.Selection
	APIKey           APIKeyInput
	BaseURL          string
	UtilityModel     string

	// ToolResultOffloadThreshold is the byte size above which Tool results are
	// offloaded; nil disables offload.
	ToolResultOffloadThreshold *int

	// Unsupported isolation must fail startup; false explicitly selects plain shells.
	SandboxShell         bool
	SandboxReadOnlyPaths []string

	Server Server
}

const defaultToolResultOffloadThreshold = 50_000
