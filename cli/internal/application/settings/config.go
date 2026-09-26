// Package settings owns the CLI's typed, validated user preferences. It knows
// nothing about Viper, files, environment variables, Cobra, or oolong; those
// adapters translate into this product model at their respective boundaries.
package settings

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

const (
	ActionSend            = "send"
	ActionNewline         = "newline"
	ActionCancelRun       = "cancel-run"
	ActionQuit            = "quit"
	ActionCommandPalette  = "command-palette"
	ActionShortcuts       = "shortcuts"
	ActionSessions        = "sessions"
	ActionTimeline        = "timeline"
	ActionSearch          = "search"
	ActionManageQueue     = "manage-queue"
	ActionChooseModel     = "choose-model"
	ActionToggleDetails   = "toggle-details"
	ActionHistoryPrevious = "history-previous"
	ActionHistoryNext     = "history-next"
	ActionNextMatch       = "next-match"
	ActionPreviousMatch   = "previous-match"
	ActionScrollPageUp    = "scroll-page-up"
	ActionScrollPageDown  = "scroll-page-down"
	ActionScrollTop       = "scroll-top"
	ActionScrollBottom    = "scroll-bottom"
	ActionExternalEditor  = "external-editor"
)

type Config struct {
	Runtime  Runtime             `json:"runtime" mapstructure:"runtime"`
	Provider string              `json:"provider" mapstructure:"provider"`
	Model    string              `json:"model"    mapstructure:"model"`
	Approval Approval            `json:"approval" mapstructure:"approval"`
	UI       UI                  `json:"ui"       mapstructure:"ui"`
	Plugins  Plugins             `json:"plugins"  mapstructure:"plugins"`
	Keys     map[string][]string `json:"keys"     mapstructure:"keys"`
}

// Runtime selects the process-owned embedded Runtime or an existing endpoint.
// Credentials are process input and never part of printable CLI preferences.
type Runtime struct {
	Endpoint string `json:"endpoint" mapstructure:"endpoint"`
}

func (r Runtime) Validate() error {
	if r.Endpoint == "" {
		return nil
	}
	endpoint, err := url.Parse(r.Endpoint)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") ||
		endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return errors.New("runtime.endpoint must be an HTTP or HTTPS URL without credentials, query, or fragment")
	}
	return nil
}

type Approval struct {
	Remember RememberPreference `json:"remember" mapstructure:"remember"`
}

// RememberPreference is the explicit configuration vocabulary. "none" maps
// to an omitted Runtime remember directive.
type RememberPreference string

const (
	RememberNone    RememberPreference = "none"
	RememberSession RememberPreference = "session"
	RememberProject RememberPreference = "project"
	RememberGlobal  RememberPreference = "global"
)

func (r RememberPreference) Scope() protocol.RememberScopeKind {
	switch r {
	case RememberSession:
		return protocol.RememberSession
	case RememberProject:
		return protocol.RememberProject
	case RememberGlobal:
		return protocol.RememberGlobal
	default:
		return ""
	}
}

type UI struct {
	Mouse            bool `json:"mouse"             mapstructure:"mouse"`
	Notifications    bool `json:"notifications"     mapstructure:"notifications"`
	ToolDetails      bool `json:"toolDetails"       mapstructure:"tool-details"`
	TranscriptRetain int  `json:"transcriptRetain"  mapstructure:"transcript-retain"`
}

type Plugins struct {
	Directories []string `json:"directories" mapstructure:"directories"`
}

func Default() Config {
	return Config{
		Approval: Approval{Remember: RememberNone},
		UI:       UI{Mouse: true, Notifications: true, ToolDetails: false, TranscriptRetain: 24},
		Keys: map[string][]string{
			ActionSend:            {"enter"},
			ActionNewline:         {"shift+enter", "alt+enter"},
			ActionCancelRun:       {"ctrl+c"},
			ActionQuit:            {"ctrl+q", "ctrl+d"},
			ActionCommandPalette:  {"ctrl+p"},
			ActionShortcuts:       {"ctrl+x"},
			ActionSessions:        {"ctrl+r"},
			ActionTimeline:        {"ctrl+g"},
			ActionSearch:          {"ctrl+f"},
			ActionManageQueue:     {"ctrl+;"},
			ActionChooseModel:     {"shift+tab"},
			ActionToggleDetails:   {"ctrl+o"},
			ActionHistoryPrevious: {"alt+up"},
			ActionHistoryNext:     {"alt+down"},
			ActionNextMatch:       {"f3"},
			ActionPreviousMatch:   {"shift+f3"},
			ActionScrollPageUp:    {"pageup"},
			ActionScrollPageDown:  {"pagedown"},
			ActionScrollTop:       {"ctrl+home"},
			ActionScrollBottom:    {"ctrl+end"},
			ActionExternalEditor:  {"ctrl+e"},
		},
	}
}

func (c Config) Validate() error {
	var problems []error
	problems = append(problems, c.Runtime.Validate())
	if _, err := c.RunOptions(); err != nil {
		problems = append(problems, err)
	}
	problems = append(problems, validateApproval(c.Approval)...)
	problems = append(problems, validateUI(c.UI)...)
	problems = append(problems, validatePluginDirectories(c.Plugins.Directories)...)
	problems = append(problems, validateKeys(c.Keys)...)
	return errors.Join(problems...)
}

func validateApproval(approval Approval) []error {
	if !slices.Contains([]RememberPreference{RememberNone, RememberSession, RememberProject, RememberGlobal}, approval.Remember) {
		return []error{fmt.Errorf("approval remember scope %q is invalid", approval.Remember)}
	}
	return nil
}

func validateUI(ui UI) []error {
	var problems []error
	if ui.TranscriptRetain < 4 || ui.TranscriptRetain > 500 {
		problems = append(problems, fmt.Errorf("ui.transcript-retain must be between 4 and 500, got %d", ui.TranscriptRetain))
	}
	return problems
}

func validatePluginDirectories(directories []string) []error {
	var problems []error
	seen := make(map[string]struct{}, len(directories))
	for _, directory := range directories {
		directory = strings.TrimSpace(directory)
		if directory == "" {
			problems = append(problems, errors.New("plugins.directories contains an empty path"))
			continue
		}
		if _, duplicate := seen[directory]; duplicate {
			problems = append(problems, fmt.Errorf("plugins.directories repeats %q", directory))
		}
		seen[directory] = struct{}{}
	}
	return problems
}

func validateKeys(keys map[string][]string) []error {
	known := Default().Keys
	var problems []error
	for _, action := range slices.Sorted(maps.Keys(known)) {
		if _, ok := keys[action]; !ok {
			problems = append(problems, fmt.Errorf("keys.%s is missing", action))
		}
	}
	for _, action := range slices.Sorted(maps.Keys(keys)) {
		bindings := keys[action]
		if _, ok := known[action]; !ok {
			problems = append(problems, fmt.Errorf("keys.%s is not a known action", action))
		}
		if len(bindings) == 0 {
			problems = append(problems, fmt.Errorf("keys.%s has no bindings", action))
		}
		for _, binding := range bindings {
			if strings.TrimSpace(binding) == "" {
				problems = append(problems, fmt.Errorf("keys.%s contains an empty binding", action))
			}
		}
	}
	return problems
}

func (c Config) RunOptions() (agent.RunOptions, error) {
	options := agent.RunOptions{Provider: c.Provider, Model: c.Model}
	return options, options.Validate()
}

func (c Config) Clone() Config {
	out := c
	out.Plugins.Directories = slices.Clone(c.Plugins.Directories)
	out.Keys = make(map[string][]string, len(c.Keys))
	for action, bindings := range c.Keys {
		out.Keys[action] = slices.Clone(bindings)
	}
	return out
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return new(*value)
}
