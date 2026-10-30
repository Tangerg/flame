// Package hook defines portable lifecycle rules and bounded event material.
// Configuration discovery, command execution, trust and final decisions belong
// to their explicit backend and Runtime owners.
package hook

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

const (
	// MaxConfigurationFileBytes bounds one complete authored hooks.json file
	// before JSON decoding or management projection.
	MaxConfigurationFileBytes int64 = 256 << 10
	// MaxHooksPerFile bounds one authored policy document.
	maxHooksPerFile = 128
	// MaxHooksPerCascade bounds the complete global + project policy resolved
	// for one workspace.
	MaxHooksPerCascade = 256
	// MaxMatcherBytes bounds one tool-name glob.
	maxMatcherBytes = 256
	// MaxActionBytes bounds one shell command or declarative injection.
	maxActionBytes = 8 << 10
	// MaxPromptBytes bounds user and sub-agent prompt material presented to one
	// command hook. Longer prompt material is projected as a marked prefix.
	MaxPromptBytes = 256 << 10
	// MaxArgumentsBytes is the lossless ceiling for canonical Tool arguments.
	// Arguments are policy input, so an oversized object is rejected rather than
	// truncated into a different action.
	MaxArgumentsBytes = 256 << 10
	// MaxResultBytes bounds Tool and sub-agent result material presented to one
	// command hook. The original result remains owned by its normal consumer.
	maxResultBytes = 128 << 10
	// MaxReasonBytes bounds failure, terminal, and descriptive material exposed
	// to one command hook.
	maxReasonBytes = 8 << 10
	// MaxTimeoutMillis prevents authored policy from replacing the Runtime's
	// bounded command lifecycle with an arbitrarily long wait.
	MaxTimeoutMillis = 5 * 60 * 1_000
)

// Event is a lifecycle point a hook can fire on.
type Event string

const (
	// PreToolUse fires before a tool runs — a hook may deny, force an approval
	// prompt, or rewrite the tool's arguments. Matched by tool name.
	PreToolUse Event = "PreToolUse"
	// PostToolUse fires after a tool produced its result — a hook may inject
	// context for the model (e.g. lint output). Matched by tool name.
	PostToolUse Event = "PostToolUse"
	// UserPromptSubmit fires when a user message opens a Run — a hook may
	// inject context or block the prompt.
	UserPromptSubmit Event = "UserPromptSubmit"
	// SessionStart fires on the first Run of a Session — a hook may inject
	// session-scoped context.
	SessionStart Event = "SessionStart"
	// PreCompact fires before model-call or Run-boundary compaction — a hook may inject
	// guidance or veto the compaction.
	PreCompact Event = "PreCompact"
	// Stop fires at Run end (any terminal) — observe-only (notify / chain).
	Stop Event = "Stop"
	// Notification fires when a run parks waiting on the user (HITL interrupt)
	// — observe-only (route to an external sink).
	Notification Event = "Notification"
)

// Scope is where a hook came from — set by the loader, not the JSON. It gates
// trust: global hooks (the user's own ~/.flame) always run; project hooks (a
// repo's .flame, which a `git clone` could carry) run only for a trusted project.
type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeProject Scope = "project"
)

// Hook is one configured hook entry. Command and Inject are alternatives: a
// Command is exec'd (real logic); an Inject is a literal context string added
// with no process spawn (the declarative fast path). A Matcher (tool-name glob)
// applies only to tool events; configuration rejects it for other events.
type Hook struct {
	Event         Event  `json:"event"`
	Matcher       string `json:"matcher,omitempty"`
	Command       string `json:"command,omitempty"`
	Inject        string `json:"inject,omitempty"`
	TimeoutMillis int    `json:"timeoutMillis,omitzero"`

	// Scope + Source are stamped by the loader (provenance + trust gating), not
	// parsed from the file.
	Scope  Scope  `json:"scope"`
	Source string `json:"source"`
}

// ErrInvalidHook reports a malformed hook configuration. Invalid policy hooks
// must fail discovery instead of silently becoming no-ops.
var ErrInvalidHook = errors.New("hooks: invalid hook")

// ErrConfigurationTooLarge reports a hooks.json document or resolved cascade
// that cannot enter the complete management and execution projections.
var ErrConfigurationTooLarge = errors.New("hooks: configuration too large")

// ErrCommandInputTooLarge reports semantic invocation material that cannot be
// projected into the private command contract without changing policy input.
var errCommandInputTooLarge = errors.New("hooks: command input too large")

// ErrInvalidCommandInput reports command material that cannot enter the
// private JSON contract without silent text replacement.
var errInvalidCommandInput = errors.New("hooks: invalid command input")

// ValidateConfigurationFileSize checks the encoded envelope before a loader
// allocates or decodes one authored policy document.
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

// ValidateHooksPerFile checks the complete entry count of one hooks.json.
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

// ValidateHookCascade checks the complete global + project policy set.
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

// Validate checks the declarative hook contract before a resolved set can be
// installed. A hook has one known lifecycle event and exactly one action;
// malformed matchers and negative timeouts are configuration errors rather
// than policies that quietly never run.
func (h Hook) Validate() error {
	switch h.Event {
	case PreToolUse, PostToolUse, UserPromptSubmit, SessionStart,
		PreCompact, Stop, Notification:
	default:
		return fmt.Errorf("%w: unsupported event %q", ErrInvalidHook, h.Event)
	}
	hasCommand := strings.TrimSpace(h.Command) != ""
	hasInject := strings.TrimSpace(h.Inject) != ""
	if hasCommand == hasInject {
		return fmt.Errorf("%w: exactly one of command or inject is required", ErrInvalidHook)
	}
	if !utf8.ValidString(h.Matcher) || !utf8.ValidString(h.Command) || !utf8.ValidString(h.Inject) {
		return fmt.Errorf("%w: text must be valid UTF-8", ErrInvalidHook)
	}
	if len(h.Matcher) > maxMatcherBytes {
		return fmt.Errorf("%w: matcher exceeds %d bytes", ErrInvalidHook, maxMatcherBytes)
	}
	if len(h.Command) > maxActionBytes || len(h.Inject) > maxActionBytes {
		return fmt.Errorf("%w: action exceeds %d bytes", ErrInvalidHook, maxActionBytes)
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
	if h.Matcher != "" {
		if h.Event != PreToolUse && h.Event != PostToolUse {
			return fmt.Errorf("%w: matcher is only valid for tool events", ErrInvalidHook)
		}
		if _, err := path.Match(h.Matcher, ""); err != nil {
			return fmt.Errorf("%w: invalid matcher %q: %w", ErrInvalidHook, h.Matcher, err)
		}
	}
	return nil
}

// Input is the semantic lifecycle payload passed to a hook runner.
type Input struct {
	Event           Event      `json:"event"`
	SessionID       string     `json:"sessionId,omitempty"`
	CWD             string     `json:"cwd,omitempty"`
	Tool            *ToolInput `json:"tool,omitzero"`
	Prompt          string     `json:"prompt,omitempty"`
	PromptTruncated bool       `json:"promptTruncated,omitzero"`
	// Reason carries a human-readable note for the observe-only events (the Stop
	// terminal detail, the Notification reason).
	Reason string `json:"reason,omitempty"`
}

// ToolInput is the tool slice of an Input for the tool events.
type ToolInput struct {
	Name            string `json:"name"`
	Arguments       string `json:"arguments,omitempty"` // canonical JSON args (Pre/PostToolUse)
	Result          string `json:"result,omitempty"`    // tool output (PostToolUse)
	ResultTruncated bool   `json:"resultTruncated,omitzero"`
}

// CommandProjection returns an ownership-isolated, bounded view for external
// command hooks. Human-readable prompt/result material may use an explicit
// marked prefix; canonical Tool arguments never truncate because doing so would
// describe a different effect to policy code.
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

// ValidateCommandMaterial is the process-boundary backstop for callers that
// bypass CommandProjection. It rejects lossy or oversized semantic input before
// JSON encoding and process creation.
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
			return fmt.Errorf("%w: %v", errInvalidCommandInput, err)
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

// Matches reports whether h applies to in.
func (h Hook) Matches(in Input) bool {
	if h.Event != in.Event {
		return false
	}
	if in.Event != PreToolUse && in.Event != PostToolUse {
		return true // non-tool events: matcher is irrelevant
	}
	if h.Matcher == "" {
		return true
	}
	name := ""
	if in.Tool != nil {
		name = in.Tool.Name
	}
	ok, err := path.Match(h.Matcher, name)
	return err == nil && ok
}

func (Event) JSONSchemaEnum() []string {
	return []string{string(PreToolUse), string(PostToolUse), string(UserPromptSubmit), string(SessionStart), string(PreCompact), string(Stop), string(Notification)}
}

func (Scope) JSONSchemaEnum() []string { return []string{string(ScopeGlobal), string(ScopeProject)} }

type CommandVerdict string

const (
	CommandAllow CommandVerdict = "allow"
	CommandDeny  CommandVerdict = "deny"
	CommandAsk   CommandVerdict = "ask"
)

func (CommandVerdict) JSONSchemaEnum() []string {
	return []string{string(CommandAllow), string(CommandDeny), string(CommandAsk)}
}
func (v CommandVerdict) Valid() bool { return v == CommandAllow || v == CommandDeny || v == CommandAsk }

type CommandDecision struct {
	Verdict          CommandVerdict `json:"verdict"`
	Reason           string         `json:"reason,omitempty"`
	InjectContext    string         `json:"injectContext,omitempty"`
	RewriteArguments string         `json:"rewriteArguments,omitempty"`
}
