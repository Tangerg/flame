package toolset

import (
	"iter"
	"reflect"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

type activityProjection func(tool.Arguments) string
type resultProjection func(tool.Arguments, tool.Result) (tool.Result, string)

type outcomeProjection uint8

const planOutcomeProjection outcomeProjection = 1

type resultProjectionContract struct {
	project    resultProjection
	resultType reflect.Type
	enums      func() map[reflect.Type][]string
}

// builtInDescriptor is the single behavioral catalog for built-in identities.
// Tool constructors own model descriptions and schemas; this catalog owns the
// cross-cutting policy and client projection attached to those definitions.
type builtInDescriptor struct {
	safety        tool.SafetyClass
	activityText  string
	activity      activityProjection
	result        resultProjectionContract
	orchestration bool
	outcome       outcomeProjection
}

func descriptors() iter.Seq2[string, builtInDescriptor] {
	return func(yield func(string, builtInDescriptor) bool) {
		for _, name := range tool.BuiltInNames() {
			ref, _ := tool.BuiltIn(name)
			descriptor, _ := descriptorFor(ref)
			if !yield(name, descriptor) {
				return
			}
		}
	}
}

func descriptorFor(ref tool.Ref) (builtInDescriptor, bool) {
	if ref.Kind() != tool.BuiltInKind {
		return builtInDescriptor{}, false
	}
	switch ref.ModelName() {
	case tool.Read:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading file"}, true
	case tool.Glob:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Finding files", result: searchResultContract()}, true
	case tool.Grep:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Searching", result: searchResultContract()}, true
	case tool.LSP:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activity: lspActivity}, true
	case tool.ReadShellOutput:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading command output"}, true
	case tool.ListSchedules:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Listing schedules"}, true
	case tool.ListSkills:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Listing Skills"}, true
	case tool.LoadSkill:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activity: loadSkillActivity}, true
	case tool.ReadSkillResource:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading a Skill resource"}, true
	case tool.SearchMemory:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Searching project memory"}, true
	case tool.SearchTools:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Loading additional tools"}, true
	case tool.AskUser:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Waiting for your answer"}, true
	case tool.EnterPlanMode:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Entering Plan mode"}, true
	case tool.ExitPlanMode:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Requesting Plan approval"}, true
	case tool.SetPlan:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Updating the Plan", outcome: planOutcomeProjection}, true
	case tool.ReadToolResult:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading omitted tool output"}, true
	case tool.DelegateTask:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activity: delegationActivity, orchestration: true}, true
	case tool.CreateGoal:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Starting an autonomous Goal"}, true
	case tool.GetGoal:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Inspecting the autonomous Goal"}, true
	case tool.ReportGoalOutcome:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reporting a Goal outcome"}, true
	case tool.ProposeSkill:
		return builtInDescriptor{safety: tool.SafetyClassSafe, activity: proposeSkillActivity}, true
	case tool.Edit:
		return builtInDescriptor{safety: tool.SafetyClassWrite, activityText: "Editing file"}, true
	case tool.ApplyPatch:
		return builtInDescriptor{safety: tool.SafetyClassWrite, activityText: "Applying a patch", result: patchResultContract()}, true
	case tool.CreateSchedule:
		return builtInDescriptor{safety: tool.SafetyClassWrite, activity: createScheduleActivity}, true
	case tool.DeleteSchedule:
		return builtInDescriptor{safety: tool.SafetyClassWrite, activityText: "Deleting a schedule"}, true
	case tool.Shell:
		return builtInDescriptor{safety: tool.SafetyClassExec, activity: shellActivity, result: commandResultContract()}, true
	case tool.StopShell:
		return builtInDescriptor{safety: tool.SafetyClassExec, activityText: "Stopping command"}, true
	case tool.WebFetch:
		return builtInDescriptor{safety: tool.SafetyClassNetwork, activityText: "Fetching a page"}, true
	case tool.WebSearch:
		return builtInDescriptor{safety: tool.SafetyClassNetwork, activityText: "Searching the web", result: webSearchResultContract()}, true
	case tool.HTTPRequest:
		return builtInDescriptor{safety: tool.SafetyClassNetwork, activity: httpActivity}, true
	default:
		return builtInDescriptor{}, false
	}
}
