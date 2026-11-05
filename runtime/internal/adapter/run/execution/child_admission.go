package execution

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	scoperun "github.com/Tangerg/scope/agent/run"
)

func (i *interactionSession) admitProcess(
	ctx context.Context,
	admission scoperun.ProcessAdmission,
) error {
	if !admission.Valid() {
		return errors.New("execution: Interaction received an invalid Process admission")
	}
	relation := admission.ProcessID()
	i.state.mu.Lock()
	deployments := i.state.deployments
	i.state.mu.Unlock()
	if deployments == nil {
		return errors.New("execution: Interaction deployments are unavailable")
	}
	if relation.Depth() == 0 {
		if admission.ProgramRef() != deployments.root.ProgramRef() {
			return errors.New("execution: Interaction root admission changed Deployment")
		}
		i.state.mu.Lock()
		defer i.state.mu.Unlock()
		if i.state.admittedProcessID.Valid() && i.state.admittedProcessID != relation {
			return errors.New("execution: Interaction root admission identity changed")
		}
		i.state.admittedProcessID = relation
		return nil
	}
	if !deployments.managedChild(admission.ProgramRef()) {
		return errors.New("execution: unmanaged child Process is outside the Runtime projection contract")
	}
	// The workflow's inner Interaction belongs to its already admitted task Run.
	if deployments.interactionChild(admission.ProgramRef()) {
		return nil
	}
	parentID, _ := relation.Parent()
	childKey, _ := relation.Key()
	identity := delegateCallIdentity{parentID: parentID, childKey: childKey}
	i.state.mu.Lock()
	managed := i.state.delegateCalls[identity]
	i.state.mu.Unlock()
	if managed == nil {
		return errors.New("execution: child admission has no durably observed Delegate call")
	}
	managed.mu.Lock()
	defer managed.mu.Unlock()
	if managed.target != admission.ProgramRef() || managed.parentRelation != parentID {
		return errors.New("execution: child admission differs from its Delegate binding")
	}
	if managed.admission.Valid() {
		if !sameManagedAdmission(managed.admission, admission) ||
			(managed.binding != (runs.ChildRunBinding{}) && managed.binding.Validate() != nil) {
			return errors.New("execution: repeated child admission changed immutable identity")
		}
		return nil
	}
	parent := i.executorMember(managed.parentRelation)
	if err := i.commitFact(ctx, parent, managed.toolStart()); err != nil {
		return fmt.Errorf("execution: commit Delegate call start: %w", err)
	}
	if err := i.rememberToolMetadata(toolResultMetadata{
		MemberID: parent.MemberID, ProcessID: managed.parentRelation.String(), Start: managed.toolStart(),
	}); err != nil {
		return err
	}
	member := runs.ExecutorMember{
		MemberID: relation.String(), ParentID: parent.MemberID,
		SpawnCallID: managed.call.ID,
	}
	request, receipt := runs.NewChildRunReservationRequest()
	if err := i.sendExecutorRequest(ctx, runs.ExecutorEvent{Member: member, Payload: request}); err != nil {
		return err
	}
	binding, err := receipt.Await(ctx)
	if err != nil {
		return err
	}
	if binding.MemberID != member.MemberID || binding.ParentRunID == "" {
		return errors.New("child Run reservation returned a different executor member")
	}
	managed.admission = admission
	managed.binding = binding
	return nil
}

func sameManagedAdmission(left, right scoperun.ProcessAdmission) bool {
	return left.Valid() && right.Valid() && left.ProcessID() == right.ProcessID() &&
		left.ProgramRef() == right.ProgramRef() &&
		left.Descriptor().Digest() == right.Descriptor().Digest() &&
		left.Budget() == right.Budget() &&
		slices.Equal(left.Capabilities().Values(), right.Capabilities().Values())
}
