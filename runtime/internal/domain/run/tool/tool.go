// Package tool defines the runtime's model-facing tool vocabulary.
package tool

import (
	"errors"
	"fmt"
	"slices"
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

// BuiltInName is one member of the closed set of Runtime built-in tools.
// Names are domain vocabulary: constructors, policy, presentation, execution,
// and recovery all refer to this single authority instead of keeping
// caller-local copies, and every consumer that keys behavior by a built-in
// keys it by this type.
type BuiltInName string

const (
	ApplyPatch        BuiltInName = "apply_patch"
	AskUser           BuiltInName = "ask_user"
	CreateGoal        BuiltInName = "create_goal"
	CreateSchedule    BuiltInName = "create_schedule"
	DeleteSchedule    BuiltInName = "delete_schedule"
	DelegateTask      BuiltInName = "delegate_task"
	Edit              BuiltInName = "edit"
	EnterPlanMode     BuiltInName = "enter_plan_mode"
	ExitPlanMode      BuiltInName = "exit_plan_mode"
	GetGoal           BuiltInName = "get_goal"
	Glob              BuiltInName = "glob"
	Grep              BuiltInName = "grep"
	HTTPRequest       BuiltInName = "http_request"
	ListSchedules     BuiltInName = "list_schedules"
	ListSkills        BuiltInName = "list_skills"
	LoadSkill         BuiltInName = "load_skill"
	LSP               BuiltInName = "lsp"
	ProposeSkill      BuiltInName = "propose_skill"
	Read              BuiltInName = "read"
	ReadShellOutput   BuiltInName = "read_shell_output"
	ReadSkillResource BuiltInName = "read_skill_resource"
	ReadToolResult    BuiltInName = "read_tool_result"
	ReportGoalOutcome BuiltInName = "report_goal_outcome"
	SearchMemory      BuiltInName = "search_memory"
	SearchTools       BuiltInName = "search_tools"
	SetPlan           BuiltInName = "set_plan"
	Shell             BuiltInName = "shell"
	StopShell         BuiltInName = "stop_shell"
	WebFetch          BuiltInName = "web_fetch"
	WebSearch         BuiltInName = "web_search"
)

// builtInNames is the set's only enumeration.
var builtInNames = []BuiltInName{
	ApplyPatch, AskUser, CreateGoal, CreateSchedule, DeleteSchedule, DelegateTask, Edit,
	EnterPlanMode, ExitPlanMode, GetGoal, Glob, Grep, HTTPRequest, ListSchedules, ListSkills,
	LoadSkill, LSP, ProposeSkill, Read, ReadShellOutput, ReadSkillResource, ReadToolResult,
	ReportGoalOutcome, SearchMemory, SearchTools, SetPlan, Shell, StopShell, WebFetch, WebSearch,
}

func BuiltInNames() []BuiltInName { return slices.Clone(builtInNames) }

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
