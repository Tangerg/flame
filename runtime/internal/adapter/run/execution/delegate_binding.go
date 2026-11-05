package execution

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
)

// delegateCallIdentity mirrors Agent Framework's documented parent-scoped ChildKey
// identity without copying any Framework tree wire into Runtime state.
type delegateCallIdentity struct {
	parentID scoperun.ProcessID
	childKey scoperun.ChildKey
}

type managedDelegateCall struct {
	mu sync.Mutex

	identity           delegateCallIdentity
	parentRelation     scoperun.ProcessID
	source             tool.Ref
	target             scoperun.ProgramRef
	call               corechat.ToolCall
	input              delegateInput
	arguments          tool.Arguments
	modelCallSequence  uint64
	toolCallIndex      uint32
	callID             runs.InvocationID
	admission          scoperun.ProcessAdmission
	binding            runs.ChildRunBinding
	childProcessID     scoperun.ProcessID
	parentToolFinished bool
	segmentProjected   bool
}

func (m *managedDelegateCall) toolStart() runs.ToolCallStarted {
	return runs.ToolCallStarted{
		CallID: m.callID.String(), ModelCallSequence: m.modelCallSequence,
		ToolCallIndex: m.toolCallIndex, SourceCallID: m.call.ID,
		ToolName: m.call.Name, Source: m.source, Arguments: m.arguments.Canonical(), SafetyClass: tool.SafetyClassExec,
	}
}

// continuedDelegateTools reopens the parent Tool attempt in the new Segment.
// Scope retains the admitted child, so continuation does not repeat admission
// and cannot obtain this start from admitProcess again. A prepared cancellation
// reopens only the parent attempts that survive its committed subtree change.
func (i *interactionSession) continuedDelegateTools() []runs.ExecutorEvent {
	i.state.mu.Lock()
	var canceled []scoperun.ProcessID
	switch i.state.boundary {
	case interactionBoundaryAcceptedPaused:
	case interactionBoundarySubtreePrepared:
		canceled = i.state.subtreeChange.canceled
	default:
		i.state.mu.Unlock()
		return nil
	}
	calls := make([]*managedDelegateCall, 0, len(i.state.delegateChildren))
	for _, managed := range i.state.delegateChildren {
		// A canceled member's own spawning Tool is still owed to its surviving
		// parent, so it reopens with its siblings. Only a Tool owned by a canceled
		// member has nobody left to answer to.
		if slices.Contains(canceled, managed.identity.parentID) {
			continue
		}
		calls = append(calls, managed)
	}
	i.state.mu.Unlock()
	type reopenedTool struct {
		member runs.ExecutorMember
		start  runs.ToolCallStarted
	}
	var reopened []reopenedTool
	for _, managed := range calls {
		managed.mu.Lock()
		finished := managed.parentToolFinished
		parent, start := managed.parentRelation, managed.toolStart()
		managed.mu.Unlock()
		if !finished {
			reopened = append(reopened, reopenedTool{member: i.executorMember(parent), start: start})
		}
	}
	slices.SortFunc(reopened, func(left, right reopenedTool) int {
		if parent := strings.Compare(left.member.MemberID, right.member.MemberID); parent != 0 {
			return parent
		}
		if call := cmp.Compare(left.start.ModelCallSequence, right.start.ModelCallSequence); call != 0 {
			return call
		}
		return cmp.Compare(left.start.ToolCallIndex, right.start.ToolCallIndex)
	})
	events := make([]runs.ExecutorEvent, len(reopened))
	for index, value := range reopened {
		events[index] = runs.ExecutorEvent{Member: value.member, Payload: value.start}
	}
	return events
}

func (i *interactionSession) installDeployments(deployments *interactionDeploymentSet) error {
	if deployments == nil || !deployments.root.Valid() {
		return errors.New("execution: install invalid Interaction deployments")
	}
	i.state.mu.Lock()
	i.state.deployments = deployments
	i.deployment = deployments.root
	i.state.mu.Unlock()
	return nil
}

// registerDelegateCalls records only model calls whose exact Deployment has a
// managed Delegate binding. Invalid delegate arguments never reach admission
// and are therefore intentionally absent from this lifecycle correlation.
func (i *interactionSession) registerDelegateCalls(
	invocation modelCallSource,
	message *corechat.Message,
) error {
	if !invocation.valid() || message == nil {
		return errors.New("execution: cannot register unattributed Delegate calls")
	}
	i.state.mu.Lock()
	deployments := i.state.deployments
	i.state.mu.Unlock()
	if deployments == nil {
		return errors.New("execution: Interaction deployments are unavailable")
	}
	toolCallIndex := uint32(0)
	for _, part := range message.Parts {
		if part.Kind != corechat.PartToolCall || part.ToolCall == nil {
			continue
		}
		call := *part.ToolCall
		target, managed := deployments.delegateTarget(invocation.program, call.Name)
		if !managed {
			toolCallIndex++
			continue
		}
		input, arguments, err := decodeDelegateCall(call, target.Descriptor())
		if err != nil {
			// Agent Framework applies the same Descriptor contract before creating a child
			// Effect and returns an ordinary Tool error for malformed input.
			toolCallIndex++
			continue
		}
		childKey, err := interaction.DelegateChildKey(invocation.sequence, call)
		if err != nil {
			return err
		}
		identity := delegateCallIdentity{
			parentID: invocation.process, childKey: childKey,
		}
		callID, err := logicalToolCallID(
			invocation.process, invocation.sequence, toolCallIndex, call.ID, call.Name,
		)
		if err != nil {
			return err
		}
		managedCall := &managedDelegateCall{
			source:   deployments.delegateSource,
			identity: identity, parentRelation: invocation.process, target: target.ProgramRef(),
			call: call, input: input, arguments: arguments,
			modelCallSequence: invocation.sequence, toolCallIndex: toolCallIndex,
			callID: callID,
		}
		i.state.mu.Lock()
		if prior := i.state.delegateCalls[identity]; prior != nil {
			i.state.mu.Unlock()
			return fmt.Errorf(
				"execution: Delegate child %q was registered more than once for parent %s",
				childKey, invocation.process,
			)
		}
		i.state.delegateCalls[identity] = managedCall
		i.state.mu.Unlock()
		toolCallIndex++
	}
	return nil
}

func decodeDelegateCall(call corechat.ToolCall, descriptor scoperun.Descriptor) (delegateInput, tool.Arguments, error) {
	erased, err := scoperun.ParsePayload([]byte(call.Arguments))
	if err != nil {
		return delegateInput{}, tool.Arguments{}, err
	}
	if err := descriptor.ValidateInput(erased); err != nil {
		return delegateInput{}, tool.Arguments{}, err
	}
	input, err := erased.Decode[delegateInput]()
	if err != nil {
		return delegateInput{}, tool.Arguments{}, err
	}
	arguments, err := tool.ParseArguments(string(erased.JSON()))
	if err != nil {
		return delegateInput{}, tool.Arguments{}, err
	}
	return input, arguments, nil
}

func (i *interactionSession) executorMember(processID scoperun.ProcessID) runs.ExecutorMember {
	member, found := i.executorMemberByProcessID(processID)
	if !found {
		return runs.ExecutorMember{}
	}
	return member
}

// The nearest managed task is the product owner. A delegated task's workflow
// and inner Interaction share one Run, while nested tasks establish new owners.
func (i *interactionSession) executorMemberByProcessID(processID scoperun.ProcessID) (runs.ExecutorMember, bool) {
	if !processID.Valid() {
		return runs.ExecutorMember{}, false
	}
	for current := processID; current.Valid(); {
		i.state.mu.Lock()
		managed := i.state.delegateChildren[current]
		root := i.state.admittedProcessID
		i.state.mu.Unlock()
		if managed != nil {
			managed.mu.Lock()
			parent, spawn := managed.parentRelation, managed.call.ID
			managed.mu.Unlock()
			parentMember, found := i.executorMemberByProcessID(parent)
			if !found {
				return runs.ExecutorMember{}, false
			}
			return runs.ExecutorMember{MemberID: current.String(), ParentID: parentMember.MemberID, SpawnCallID: spawn}, true
		}
		if current == root {
			return runs.ExecutorMember{MemberID: root.String()}, true
		}
		parent, child := current.Parent()
		if !child {
			break
		}
		current = parent
	}
	return runs.ExecutorMember{}, false
}

func (i *interactionSession) toolCallMember(processID scoperun.ProcessID) (runs.ExecutorMember, bool) {
	member, found := i.executorMemberByProcessID(processID)
	if !found {
		return member, false
	}
	owner, err := scoperun.ParseProcessID(member.MemberID)
	if err != nil {
		return runs.ExecutorMember{}, false
	}
	i.state.mu.Lock()
	managed := i.state.delegateChildren[owner]
	i.state.mu.Unlock()
	if managed != nil {
		managed.mu.Lock()
		retired := managed.segmentProjected
		managed.mu.Unlock()
		if retired {
			return runs.ExecutorMember{}, false
		}
	}
	return member, true
}
