// Package tool defines the runtime's model-facing tool vocabulary.
package tool

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/Tangerg/scope/core/chat"
)

// ErrInvalidDefinition reports registered Tool metadata that cannot be used as
// one stable model- and client-facing capability definition.
var ErrInvalidDefinition = errors.New("tool: invalid definition")

// Group identifies the model-facing Tool surface assigned to one Interaction
// deployment layer.
type Group string

const (
	// GroupRoot is the complete product-tool surface used by the root Agent.
	GroupRoot Group = "root"
	// GroupDelegated is the bounded surface used by delegated Agents.
	GroupDelegated Group = "delegated"
)

// Valid reports whether g names one supported Tool surface.
func (g Group) Valid() bool { return g == GroupRoot || g == GroupDelegated }

// Runtime-owned model-facing tool identities. Names are domain vocabulary:
// constructors, policy, presentation, execution, and recovery all refer to
// this single authority instead of keeping caller-local copies.
const (
	ApplyPatch          = "apply_patch"
	AskUser             = "ask_user"
	CreateGoal          = "create_goal"
	CreateSchedule      = "create_schedule"
	DeleteSchedule      = "delete_schedule"
	DelegateTask        = "delegate_task"
	Edit                = "edit"
	EnterPlanMode       = "enter_plan_mode"
	ExitPlanMode        = "exit_plan_mode"
	GetGoal             = "get_goal"
	Glob                = "glob"
	Grep                = "grep"
	HTTPRequest         = "http_request"
	ListSchedules       = "list_schedules"
	ListSkills          = "list_skills"
	LoadSkill           = "load_skill"
	LSP                 = "lsp"
	ProposeSkill        = "propose_skill"
	Read                = "read"
	ReadShellOutput     = "read_shell_output"
	ReadSkillResource   = "read_skill_resource"
	ReadToolResult      = "read_tool_result"
	ReportGoalOutcome   = "report_goal_outcome"
	SearchConversations = "search_conversations"
	SearchMemory        = "search_memory"
	SearchTools         = "search_tools"
	SetPlan             = "set_plan"
	Shell               = "shell"
	StopShell           = "stop_shell"
	WebFetch            = "web_fetch"
	WebSearch           = "web_search"
)

// Tool projects a Scope definition with the Runtime-owned safety class.
type Tool struct {
	chat.ToolDefinition
	SafetyClass SafetyClass
}

// Validate checks the metadata invariants shared by every Tool catalog.
func (t Tool) Validate() error {
	if err := t.ToolDefinition.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDefinition, err)
	}
	if !utf8.ValidString(t.Description) {
		return fmt.Errorf("%w: Tool %q description is not valid UTF-8", ErrInvalidDefinition, t.Name)
	}
	if !t.SafetyClass.Valid() {
		return fmt.Errorf("%w: Tool %q has unknown safety class %q", ErrInvalidDefinition, t.Name, t.SafetyClass)
	}
	return nil
}

// SafetyClass classifies how aggressively the runtime gates a tool call
// behind an approval prompt. Its values are also the durable vocabulary used
// by run checkpoints; the empty value is invalid rather than silently safe.
type SafetyClass string

const (
	// SafetyClassSafe — read-only, no side effects (read, grep, glob,
	// skill). Never prompts. Network-reaching tools are not safe even when they
	// only read remote state.
	SafetyClassSafe SafetyClass = "safe"
	// SafetyClassWrite — writes files in cwd. Prompts in `safe` mode.
	SafetyClassWrite SafetyClass = "write"
	// SafetyClassExec — executes arbitrary commands (Shell). Prompts
	// in `safe` and `balanced` modes.
	SafetyClassExec SafetyClass = "exec"
	// SafetyClassNetwork — reaches off-host network. Safe/Plan gate it;
	// Balanced allows explicitly configured built-ins.
	SafetyClassNetwork SafetyClass = "network"
)
