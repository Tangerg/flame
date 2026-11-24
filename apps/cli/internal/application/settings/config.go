package settings

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
)

const defaultTranscriptRetain = 24

type Action string

const (
	ActionSend            Action = "send"
	ActionNewline         Action = "newline"
	ActionCancelRun       Action = "cancel-run"
	ActionQuit            Action = "quit"
	ActionCommandPalette  Action = "command-palette"
	ActionShortcuts       Action = "shortcuts"
	ActionSessions        Action = "sessions"
	ActionTimeline        Action = "timeline"
	ActionSearch          Action = "search"
	ActionManageQueue     Action = "manage-queue"
	ActionChooseModel     Action = "choose-model"
	ActionToggleDetails   Action = "toggle-details"
	ActionHistoryPrevious Action = "history-previous"
	ActionHistoryNext     Action = "history-next"
	ActionNextMatch       Action = "next-match"
	ActionPreviousMatch   Action = "previous-match"
	ActionScrollPageUp    Action = "scroll-page-up"
	ActionScrollPageDown  Action = "scroll-page-down"
	ActionScrollTop       Action = "scroll-top"
	ActionScrollBottom    Action = "scroll-bottom"
	ActionExternalEditor  Action = "external-editor"
)

func (r Runtime) AuthoringPartition() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	if r.Mode == RuntimeEmbedded {
		return "", nil
	}
	endpoint, err := httporigin.ParseEndpoint(r.Endpoint)
	if err != nil {
		return "", err
	}
	return endpoint.CanonicalBase(), nil
}

type Config struct {
	Runtime  Runtime             `json:"runtime" mapstructure:"runtime"`
	Provider string              `json:"provider" mapstructure:"provider"`
	Model    string              `json:"model"    mapstructure:"model"`
	Approval Approval            `json:"approval" mapstructure:"approval"`
	UI       UI                  `json:"ui"       mapstructure:"ui"`
	Plugins  Plugins             `json:"plugins"  mapstructure:"plugins"`
	Keys     map[Action][]string `json:"keys"     mapstructure:"keys"`
}

type Runtime struct {
	Mode     RuntimeMode `json:"mode" mapstructure:"mode"`
	Endpoint string      `json:"endpoint" mapstructure:"endpoint"`
}

type RuntimeMode string

const (
	RuntimeRemote   RuntimeMode = "remote"
	RuntimeEmbedded RuntimeMode = "embedded"
)

func (r Runtime) Validate() error {
	if r.Mode == RuntimeEmbedded {
		return nil
	}
	if r.Mode != RuntimeRemote {
		return errors.New("runtime.mode must be remote or embedded")
	}
	endpoint, err := httporigin.ParseEndpoint(r.Endpoint)
	if err != nil || endpoint.HasQuery() || endpoint.HasFragment() {
		return errors.New("runtime.endpoint must be an HTTP or HTTPS URL without credentials, query, or fragment")
	}
	return nil
}

type Approval struct {
	Remember RememberPreference `json:"remember" mapstructure:"remember"`
}

type RememberPreference string

const (
	RememberNone      RememberPreference = "none"
	RememberSession   RememberPreference = "session"
	RememberWorkspace RememberPreference = "workspace"
	RememberGlobal    RememberPreference = "global"
)

type UI struct {
	Mouse            bool `json:"mouse"             mapstructure:"mouse"`
	Notifications    bool `json:"notifications"     mapstructure:"notifications"`
	ToolDetails      bool `json:"toolDetails"       mapstructure:"tool-details"`
	TranscriptRetain int  `json:"transcriptRetain"  mapstructure:"transcript-retain"`
}

type Plugins struct {
	Directories []string `json:"directories" mapstructure:"directories"`
}

// User-sideloaded plugins live in the home authored directory, which does not move
// with a relocated product root.
func DefaultPluginDirectory(home string) string {
	return filepath.Join(home, identity.AuthoredDirectory, "plugins")
}

func Default() Config {
	return Config{
		Runtime:  Runtime{Mode: RuntimeRemote, Endpoint: localruntime.DefaultOrigin},
		Approval: Approval{Remember: RememberNone},
		UI:       UI{Mouse: true, Notifications: true, ToolDetails: false, TranscriptRetain: defaultTranscriptRetain},
		Keys: map[Action][]string{
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
	if !slices.Contains([]RememberPreference{RememberNone, RememberSession, RememberWorkspace, RememberGlobal}, approval.Remember) {
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

func validateKeys(keys map[Action][]string) []error {
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

func (c Config) RunOptions() (authoring.RunOptions, error) {
	options := authoring.RunOptions{Provider: c.Provider, Model: c.Model}
	return options, options.Validate()
}

func (c Config) Clone() Config {
	out := c
	out.Plugins.Directories = slices.Clone(c.Plugins.Directories)
	out.Keys = make(map[Action][]string, len(c.Keys))
	for action, bindings := range c.Keys {
		out.Keys[action] = slices.Clone(bindings)
	}
	return out
}
