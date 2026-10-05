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

// Built-in membership belongs to the domain; this catalog is keyed by it and a
// test holds it to exactly the domain's set. It supplies policy and client
// presentation; tool constructors own model descriptions and schemas.
type builtInDescriptor struct {
	safety        tool.SafetyClass
	activityText  string
	activity      activityProjection
	result        resultProjectionContract
	orchestration bool
	outcome       outcomeProjection
}

var builtInDescriptors = map[tool.BuiltInName]builtInDescriptor{
	tool.Read:              builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading file"},
	tool.Glob:              builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Finding files", result: searchResultContract()},
	tool.Grep:              builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Searching", result: searchResultContract()},
	tool.LSP:               builtInDescriptor{safety: tool.SafetyClassSafe, activity: lspActivity},
	tool.ReadShellOutput:   builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading command output"},
	tool.ListSchedules:     builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Listing schedules"},
	tool.ListSkills:        builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Listing Skills"},
	tool.LoadSkill:         builtInDescriptor{safety: tool.SafetyClassSafe, activity: loadSkillActivity},
	tool.ReadSkillResource: builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading a Skill resource"},
	tool.SearchMemory:      builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Searching project memory"},
	tool.SearchTools:       builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Loading additional tools"},
	tool.AskUser:           builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Waiting for your answer"},
	tool.EnterPlanMode:     builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Entering Plan mode"},
	tool.ExitPlanMode:      builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Requesting Plan approval"},
	tool.SetPlan:           builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Updating the Plan", outcome: planOutcomeProjection},
	tool.ReadToolResult:    builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading omitted tool output"},
	tool.DelegateTask:      builtInDescriptor{safety: tool.SafetyClassSafe, activity: delegationActivity, orchestration: true},
	tool.CreateGoal:        builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Starting an autonomous Goal"},
	tool.GetGoal:           builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Inspecting the autonomous Goal"},
	tool.ReportGoalOutcome: builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reporting a Goal outcome"},
	tool.ProposeSkill:      builtInDescriptor{safety: tool.SafetyClassSafe, activity: proposeSkillActivity},
	tool.Edit:              builtInDescriptor{safety: tool.SafetyClassWrite, activityText: "Editing file"},
	tool.ApplyPatch:        builtInDescriptor{safety: tool.SafetyClassWrite, activityText: "Applying a patch", result: patchResultContract()},
	tool.CreateSchedule:    builtInDescriptor{safety: tool.SafetyClassWrite, activity: createScheduleActivity},
	tool.DeleteSchedule:    builtInDescriptor{safety: tool.SafetyClassWrite, activityText: "Deleting a schedule"},
	tool.Shell:             builtInDescriptor{safety: tool.SafetyClassExec, activity: shellActivity, result: commandResultContract()},
	tool.StopShell:         builtInDescriptor{safety: tool.SafetyClassExec, activityText: "Stopping command"},
	tool.WebFetch:          builtInDescriptor{safety: tool.SafetyClassNetwork, activityText: "Fetching a page"},
	tool.WebSearch:         builtInDescriptor{safety: tool.SafetyClassNetwork, activityText: "Searching the web", result: webSearchResultContract()},
	tool.HTTPRequest:       builtInDescriptor{safety: tool.SafetyClassNetwork, activity: httpActivity},
}

func descriptors() iter.Seq2[tool.BuiltInName, builtInDescriptor] {
	return func(yield func(tool.BuiltInName, builtInDescriptor) bool) {
		for _, name := range tool.BuiltInNames() {
			if !yield(name, builtInDescriptors[name]) {
				return
			}
		}
	}
}
func descriptorFor(ref tool.Ref) (builtInDescriptor, bool) {
	name, ok := ref.BuiltIn()
	if !ok {
		return builtInDescriptor{}, false
	}
	descriptor, found := builtInDescriptors[name]
	return descriptor, found
}
