package hook

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

// ConfigurationPath is relative to the home or repository directory whose hooks it declares.
const ConfigurationPath = identity.AuthoredDirectory + "/hooks.json"

const (
	MaxConfigurationFileBytes int64 = 256 << 10
	maxHooksPerFile                 = 128
	MaxHooksPerCascade              = 256
	maxMatcherBytes                 = 256
	maxActionBytes                  = 8 << 10
	MaxPromptBytes                  = 256 << 10
	// Arguments are policy input, so an oversized object is rejected rather than
	// truncated into a different action.
	MaxArgumentsBytes = 256 << 10
	maxResultBytes    = 128 << 10
	maxReasonBytes    = 8 << 10
	MaxTimeoutMillis  = 5 * 60 * 1_000
)

type Event string

const (
	PreToolUse       Event = "PreToolUse"
	PostToolUse      Event = "PostToolUse"
	UserPromptSubmit Event = "UserPromptSubmit"
	SessionStart     Event = "SessionStart"
	PreCompact       Event = "PreCompact"
	Stop             Event = "Stop"
	Notification     Event = "Notification"
)

type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeRepository Scope = "repository"
)

type FailurePolicy string

const (
	FailureAdvisory FailurePolicy = "advisory"
	FailureRequired FailurePolicy = "required"
)

func (FailurePolicy) Values() []string {
	return []string{string(FailureAdvisory), string(FailureRequired)}
}

func (p FailurePolicy) JSONSchemaEnum() []string { return p.Values() }

func (p FailurePolicy) Effective() FailurePolicy {
	if p == "" {
		return FailureAdvisory
	}
	return p
}

func (p FailurePolicy) validate(event Event, hasCommand bool) error {
	if p == "" {
		return nil
	}
	if !slices.Contains(p.Values(), string(p)) {
		return fmt.Errorf("%w: unsupported failurePolicy %q", ErrInvalidHook, p)
	}
	if !hasCommand {
		return fmt.Errorf("%w: failurePolicy is only valid for command hooks", ErrInvalidHook)
	}
	if p == FailureRequired && !slices.Contains(RequiredFailureEvents(), event) {
		return fmt.Errorf("%w: required failurePolicy needs an event that can reject its pending action", ErrInvalidHook)
	}
	return nil
}

func RequiredFailureEvents() []Event { return []Event{PreToolUse, UserPromptSubmit, PreCompact} }

type Hook struct {
	Event         Event         `json:"event"`
	Matcher       string        `json:"matcher,omitempty"`
	Command       string        `json:"command,omitempty"`
	Inject        string        `json:"inject,omitempty"`
	TimeoutMillis int           `json:"timeoutMillis,omitzero"`
	FailurePolicy FailurePolicy `json:"failurePolicy,omitempty"`

	Scope  Scope  `json:"scope"`
	Source string `json:"source"`
}

var ErrInvalidHook = errors.New("hooks: invalid hook")

var ErrConfigurationTooLarge = errors.New("hooks: configuration too large")

var errCommandInputTooLarge = errors.New("hooks: command input too large")

var errInvalidCommandInput = errors.New("hooks: invalid command input")

func ValidateConfigurationFileSize(size int64) error {
	if size < 0 || size > MaxConfigurationFileBytes {
		return fmt.Errorf(
			"%w: file uses %d bytes, maximum %d",
			ErrConfigurationTooLarge,
			size,
			MaxConfigurationFileBytes,
		)
	}
	return nil
}

func ValidateHooksPerFile(count int) error {
	if count < 0 || count > maxHooksPerFile {
		return fmt.Errorf(
			"%w: file contains %d hooks, maximum %d",
			ErrConfigurationTooLarge,
			count,
			maxHooksPerFile,
		)
	}
	return nil
}

func ValidateHookCascade(count int) error {
	if count < 0 || count > MaxHooksPerCascade {
		return fmt.Errorf(
			"%w: cascade contains %d hooks, maximum %d",
			ErrConfigurationTooLarge,
			count,
			MaxHooksPerCascade,
		)
	}
	return nil
}

func (h Hook) Validate() error {
	if !h.Event.Valid() {
		return fmt.Errorf("%w: unsupported event %q", ErrInvalidHook, h.Event)
	}
	hasCommand := strings.TrimSpace(h.Command) != ""
	hasInject := strings.TrimSpace(h.Inject) != ""
	if hasCommand == hasInject {
		return fmt.Errorf("%w: exactly one of command or inject is required", ErrInvalidHook)
	}
	if err := h.validateText(); err != nil {
		return err
	}
	if h.TimeoutMillis < 0 || h.TimeoutMillis > MaxTimeoutMillis {
		return fmt.Errorf(
			"%w: timeoutMillis must be between 0 and %d",
			ErrInvalidHook,
			MaxTimeoutMillis,
		)
	}
	if hasInject && h.TimeoutMillis != 0 {
		return fmt.Errorf("%w: timeoutMillis is only valid for command hooks", ErrInvalidHook)
	}
	if err := h.FailurePolicy.validate(h.Event, hasCommand); err != nil {
		return err
	}
	return h.validateMatcher()
}

func (h Hook) validateText() error {
	if !utf8.ValidString(h.Matcher) || !utf8.ValidString(h.Command) || !utf8.ValidString(h.Inject) {
		return fmt.Errorf("%w: text must be valid UTF-8", ErrInvalidHook)
	}
	if len(h.Matcher) > maxMatcherBytes {
		return fmt.Errorf("%w: matcher exceeds %d bytes", ErrInvalidHook, maxMatcherBytes)
	}
	if len(h.Command) > maxActionBytes || len(h.Inject) > maxActionBytes {
		return fmt.Errorf("%w: action exceeds %d bytes", ErrInvalidHook, maxActionBytes)
	}
	return nil
}

func (h Hook) validateMatcher() error {
	if h.Matcher == "" {
		return nil
	}
	if h.Event != PreToolUse && h.Event != PostToolUse {
		return fmt.Errorf("%w: matcher is only valid for tool events", ErrInvalidHook)
	}
	if _, err := path.Match(h.Matcher, ""); err != nil {
		return fmt.Errorf("%w: invalid matcher %q: %w", ErrInvalidHook, h.Matcher, err)
	}
	return nil
}

type Input struct {
	Event           Event      `json:"event"`
	SessionID       string     `json:"sessionId,omitempty"`
	CWD             string     `json:"cwd,omitempty"`
	Tool            *ToolInput `json:"tool,omitzero"`
	Prompt          string     `json:"prompt,omitempty"`
	PromptTruncated bool       `json:"promptTruncated,omitzero"`
	Reason          string     `json:"reason,omitempty"`
}

type ToolInput struct {
	Name            string `json:"name"`
	Arguments       string `json:"arguments,omitempty"`
	Result          string `json:"result,omitempty"`
	ResultTruncated bool   `json:"resultTruncated,omitzero"`
}

// Canonical Tool arguments never truncate: a truncated object would describe a
// different effect to policy code.
func (i Input) CommandProjection() (Input, error) {
	out := i
	out.Prompt, out.PromptTruncated = boundedCommandText(i.Prompt, MaxPromptBytes, i.PromptTruncated)
	out.Reason, _ = boundedCommandText(i.Reason, maxReasonBytes, false)
	if i.Tool != nil {
		if len(i.Tool.Arguments) > MaxArgumentsBytes {
			return Input{}, fmt.Errorf(
				"%w: tool arguments use %d bytes, maximum %d",
				errCommandInputTooLarge,
				len(i.Tool.Arguments),
				MaxArgumentsBytes,
			)
		}
		tool := *i.Tool
		tool.Result, tool.ResultTruncated = boundedCommandText(
			i.Tool.Result,
			maxResultBytes,
			i.Tool.ResultTruncated,
		)
		out.Tool = &tool
	}
	if err := out.ValidateCommandMaterial(); err != nil {
		return Input{}, err
	}
	return out, nil
}

func (i Input) ValidateCommandMaterial() error {
	if len(i.Prompt) > MaxPromptBytes || len(i.Reason) > maxReasonBytes {
		return errCommandInputTooLarge
	}
	values := []string{string(i.Event), i.SessionID, i.CWD, i.Prompt, i.Reason}
	if i.Tool != nil {
		if len(i.Tool.Arguments) > MaxArgumentsBytes || len(i.Tool.Result) > maxResultBytes {
			return errCommandInputTooLarge
		}
		values = append(values, i.Tool.Name, i.Tool.Arguments, i.Tool.Result)
	}
	if i.SessionID != "" {
		if err := identity.ValidateSession(i.SessionID); err != nil {
			return fmt.Errorf("%w: %w", errInvalidCommandInput, err)
		}
	}
	for _, value := range values {
		if !utf8.ValidString(value) {
			return fmt.Errorf("%w: material must be valid UTF-8", errInvalidCommandInput)
		}
	}
	return nil
}

func boundedCommandText(value string, limit int, alreadyTruncated bool) (string, bool) {
	if limit <= 0 {
		return "", alreadyTruncated || value != ""
	}
	truncated := alreadyTruncated || len(value) > limit
	if len(value) > limit {
		value = value[:limit]
	}
	valid := strings.ToValidUTF8(value, "�")
	if valid != value {
		truncated = true
	}
	if len(valid) > limit {
		valid = valid[:limit]
		for len(valid) > 0 && !utf8.ValidString(valid) {
			valid = valid[:len(valid)-1]
		}
		truncated = true
	}
	return valid, truncated
}

func (e Event) Valid() bool { return slices.Contains(e.JSONSchemaEnum(), string(e)) }

func (Event) JSONSchemaEnum() []string {
	return []string{string(PreToolUse), string(PostToolUse), string(UserPromptSubmit), string(SessionStart), string(PreCompact), string(Stop), string(Notification)}
}

func (Scope) JSONSchemaEnum() []string { return []string{string(ScopeGlobal), string(ScopeRepository)} }

type CommandVerdict string

const (
	CommandAllow CommandVerdict = "allow"
	CommandDeny  CommandVerdict = "deny"
	CommandAsk   CommandVerdict = "ask"
)

func (CommandVerdict) JSONSchemaEnum() []string {
	return []string{string(CommandAllow), string(CommandDeny), string(CommandAsk)}
}

func (v CommandVerdict) Valid() bool { return slices.Contains(v.JSONSchemaEnum(), string(v)) }

type CommandDecision struct {
	Verdict          CommandVerdict `json:"verdict"`
	Reason           string         `json:"reason,omitempty"`
	InjectContext    string         `json:"injectContext,omitempty"`
	RewriteArguments string         `json:"rewriteArguments,omitempty"`
}
