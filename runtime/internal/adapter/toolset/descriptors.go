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
	orchestration bool
	outcome       outcomeProjection
}

var builtInDescriptors = map[builtincontract.Name]builtInDescriptor{
	builtincontract.Read:              builtInDescriptor{safety: tool.SafetyClassSafe},
	builtincontract.Glob:              builtInDescriptor{safety: tool.SafetyClassSafe},
	builtincontract.Grep:              builtInDescriptor{safety: tool.SafetyClassSafe},
	builtincontract.ReadShellOutput:   builtInDescriptor{safety: tool.SafetyClassSafe},
	builtincontract.ListSkills:        builtInDescriptor{safety: tool.SafetyClassSafe},
	builtincontract.LoadSkill:         builtInDescriptor{safety: tool.SafetyClassSafe},
	builtincontract.ReadSkillResource: builtInDescriptor{safety: tool.SafetyClassSafe},
	builtincontract.SearchTools:       builtInDescriptor{safety: tool.SafetyClassSafe},
	builtincontract.AskUser:           builtInDescriptor{safety: tool.SafetyClassSafe},
	builtincontract.EnterPlanMode:     builtInDescriptor{safety: tool.SafetyClassSafe},
	builtincontract.ExitPlanMode:      builtInDescriptor{safety: tool.SafetyClassSafe},
	builtincontract.SetPlan:           builtInDescriptor{safety: tool.SafetyClassSafe, outcome: planOutcomeProjection},
	builtincontract.ReadToolResult:    builtInDescriptor{safety: tool.SafetyClassSafe},
	builtincontract.DelegateTask:      builtInDescriptor{safety: tool.SafetyClassSafe, orchestration: true},
	builtincontract.CreateGoal:        builtInDescriptor{safety: tool.SafetyClassSafe},
	builtincontract.GetGoal:           builtInDescriptor{safety: tool.SafetyClassSafe},
	builtincontract.ReportGoalOutcome: builtInDescriptor{safety: tool.SafetyClassSafe},
	builtincontract.ProposeSkill:      builtInDescriptor{safety: tool.SafetyClassSafe},
	builtincontract.Edit:              builtInDescriptor{safety: tool.SafetyClassWrite},
	builtincontract.ApplyPatch:        builtInDescriptor{safety: tool.SafetyClassWrite},
	builtincontract.Shell:             builtInDescriptor{safety: tool.SafetyClassExec},
	builtincontract.StopShell:         builtInDescriptor{safety: tool.SafetyClassExec},
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
