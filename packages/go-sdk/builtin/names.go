// Package builtin declares Runtime-owned built-in tool identities and value contracts.
package builtin

import "slices"

// Name identifies a current Runtime-owned tool.
type Name string

const (
	ApplyPatch        Name = "apply_patch"
	AskUser           Name = "ask_user"
	CreateGoal        Name = "create_goal"
	DelegateTask      Name = "delegate_task"
	Edit              Name = "edit"
	EnterPlanMode     Name = "enter_plan_mode"
	ExitPlanMode      Name = "exit_plan_mode"
	GetGoal           Name = "get_goal"
	Glob              Name = "glob"
	Grep              Name = "grep"
	ListSkills        Name = "list_skills"
	LoadSkill         Name = "load_skill"
	ProposeSkill      Name = "propose_skill"
	Read              Name = "read"
	ReadShellOutput   Name = "read_shell_output"
	ReadSkillResource Name = "read_skill_resource"
	ReadToolResult    Name = "read_tool_result"
	ReportGoalOutcome Name = "report_goal_outcome"
	SearchTools       Name = "search_tools"
	SetPlan           Name = "set_plan"
	Shell             Name = "shell"
	StopShell         Name = "stop_shell"
)

// names is the set's only enumeration.
var names = []Name{
	ApplyPatch, AskUser, CreateGoal, DelegateTask, Edit,
	EnterPlanMode, ExitPlanMode, GetGoal, Glob, Grep, ListSkills,
	LoadSkill, ProposeSkill, Read, ReadShellOutput, ReadSkillResource, ReadToolResult,
	ReportGoalOutcome, SearchTools, SetPlan, Shell, StopShell,
}

func Names() []Name { return slices.Clone(names) }

func (n Name) Valid() bool { return slices.Contains(names, n) }
