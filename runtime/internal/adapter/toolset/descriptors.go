package toolset

import (
	"iter"
	"maps"
	"reflect"
	"slices"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
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

// This catalog owns current executable metadata. Durable identities can outlive
// their executables; retaining an old reference cannot make it dispatchable.
type builtInDescriptor struct {
	safety        tool.SafetyClass
	activityText  string
	activity      activityProjection
	orchestration bool
	outcome       outcomeProjection
}

var builtInDescriptors = map[builtincontract.Name]builtInDescriptor{
	builtincontract.Read:              builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading file"},
	builtincontract.Glob:              builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Finding files"},
	builtincontract.Grep:              builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Searching"},
	builtincontract.ReadShellOutput:   builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading command output"},
	builtincontract.ListSkills:        builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Listing Skills"},
	builtincontract.LoadSkill:         builtInDescriptor{safety: tool.SafetyClassSafe, activity: loadSkillActivity},
	builtincontract.ReadSkillResource: builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading a Skill resource"},
	builtincontract.SearchTools:       builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Loading additional tools"},
	builtincontract.AskUser:           builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Waiting for your answer"},
	builtincontract.EnterPlanMode:     builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Entering Plan mode"},
	builtincontract.ExitPlanMode:      builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Requesting Plan approval"},
	builtincontract.SetPlan:           builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Updating the Plan", outcome: planOutcomeProjection},
	builtincontract.ReadToolResult:    builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reading omitted tool output"},
	builtincontract.DelegateTask:      builtInDescriptor{safety: tool.SafetyClassSafe, activity: delegationActivity, orchestration: true},
	builtincontract.CreateGoal:        builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Starting an autonomous Goal"},
	builtincontract.GetGoal:           builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Inspecting the autonomous Goal"},
	builtincontract.ReportGoalOutcome: builtInDescriptor{safety: tool.SafetyClassSafe, activityText: "Reporting a Goal outcome"},
	builtincontract.ProposeSkill:      builtInDescriptor{safety: tool.SafetyClassSafe, activity: proposeSkillActivity},
	builtincontract.Edit:              builtInDescriptor{safety: tool.SafetyClassWrite, activityText: "Editing file"},
	builtincontract.ApplyPatch:        builtInDescriptor{safety: tool.SafetyClassWrite, activityText: "Applying a patch"},
	builtincontract.Shell:             builtInDescriptor{safety: tool.SafetyClassExec, activity: shellActivity},
	builtincontract.StopShell:         builtInDescriptor{safety: tool.SafetyClassExec, activityText: "Stopping command"},
}

// resultContractFor binds the projection implementing a built-in's declared
// result shape; the shape itself is owned by the builtin contract.
func resultContractFor(name builtincontract.Name) (resultProjectionContract, bool) {
	switch name.ResultShape() {
	case extension.ResultShapeCommand:
		return commandResultContract(), true
	case extension.ResultShapeSearch:
		return searchResultContract(), true
	case extension.ResultShapePatch:
		return patchResultContract(), true
	default:
		return resultProjectionContract{}, false
	}
}

func descriptors() iter.Seq2[builtincontract.Name, builtInDescriptor] {
	return func(yield func(builtincontract.Name, builtInDescriptor) bool) {
		for _, name := range slices.Sorted(maps.Keys(builtInDescriptors)) {
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
