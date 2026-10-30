package builtin

import "github.com/Tangerg/flame/packages/go-sdk/extension"

// ToolCategory groups built-in tools for presentation, so every client renders a
// built-in call from one grouping. Tools outside the built-in set carry none.
type ToolCategory string

const (
	CategoryShell  ToolCategory = "shell"
	CategoryEdit   ToolCategory = "edit"
	CategoryRead   ToolCategory = "read"
	CategorySearch ToolCategory = "search"
	CategoryTask   ToolCategory = "task"
)

func (ToolCategory) Values() []string {
	return []string{string(CategoryShell), string(CategoryEdit), string(CategoryRead), string(CategorySearch), string(CategoryTask)}
}

type presentation struct {
	category ToolCategory
	shape    extension.ResultShape
}

// presentations is the one table of built-in presentation facts. A shape is
// declared only where the tool's result is that shape's contract type.
var presentations = map[Name]presentation{
	ApplyPatch:        {category: CategoryEdit, shape: extension.ResultShapePatch},
	AskUser:           {category: CategoryTask},
	CreateGoal:        {category: CategoryTask},
	DelegateTask:      {category: CategoryTask},
	Edit:              {category: CategoryEdit},
	EnterPlanMode:     {category: CategoryTask},
	ExitPlanMode:      {category: CategoryTask},
	GetGoal:           {category: CategoryTask},
	Glob:              {category: CategorySearch, shape: extension.ResultShapeSearch},
	Grep:              {category: CategorySearch, shape: extension.ResultShapeSearch},
	ListSkills:        {category: CategoryTask},
	LoadSkill:         {category: CategoryTask},
	ProposeSkill:      {category: CategoryTask},
	Read:              {category: CategoryRead},
	ReadShellOutput:   {category: CategoryShell},
	ReadSkillResource: {category: CategoryRead},
	ReadToolResult:    {category: CategoryRead},
	ReportGoalOutcome: {category: CategoryTask},
	SearchTools:       {category: CategorySearch},
	SetPlan:           {category: CategoryTask},
	Shell:             {category: CategoryShell, shape: extension.ResultShapeCommand},
	StopShell:         {category: CategoryShell},
}

// Category is empty for a name outside the built-in set.
func (n Name) Category() ToolCategory { return presentations[n].category }

// ResultShape is empty when the tool's result has no shared shape contract.
func (n Name) ResultShape() extension.ResultShape { return presentations[n].shape }
