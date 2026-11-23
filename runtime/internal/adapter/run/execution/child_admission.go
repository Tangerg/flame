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
	deployments, err := i.state.boundDeployments()
	if err != nil {
		return err
	}
	if relation.Depth() == 0 {
		if admission.ProgramRef() != deployments.root.ProgramRef() {
			return errors.New("execution: Interaction root admission changed Deployment")
		}
		if relation != i.rootID {
			return errors.New("execution: Interaction root differs from the Run's start identity")
		}
		return nil
	}
	if !deployments.managedChild(admission.ProgramRef()) {
		return errors.New("execution: unmanaged child Process is outside the Runtime projection contract")
	}
	if deployments.interactionChild(admission.ProgramRef()) {
		return nil
	}
	return i.admitDelegateChild(ctx, admission)
}

func (i *interactionSession) admitDelegateChild(ctx context.Context, admission scoperun.ProcessAdmission) error {
	relation := admission.ProcessID()
	parentID, _ := relation.Parent()
	childKey, _ := relation.Key()
	i.state.mu.Lock()
	managed := i.state.delegateCalls[delegateCallIdentity{parentID: parentID, childKey: childKey}]
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
	binding, err := i.reserveChildRun(ctx, managed, relation)
	if err != nil {
		return err
	}
	managed.admission = admission
	managed.binding = binding
	return nil
}

func (i *interactionSession) reserveChildRun(ctx context.Context, managed *managedDelegateCall, relation scoperun.ProcessID) (runs.ChildRunBinding, error) {
	parent := i.executorMember(managed.parentRelation)
	if err := i.commitFact(ctx, parent, managed.toolStart()); err != nil {
		return runs.ChildRunBinding{}, fmt.Errorf("execution: commit Delegate call start: %w", err)
	}
	capture := toolCallCapture{MemberID: parent.MemberID, ProcessID: managed.parentRelation, Start: managed.toolStart()}
	if err := i.rememberToolMetadata(toolCallMetadata{toolCallCapture: capture}); err != nil {
		return runs.ChildRunBinding{}, err
	}
	member := runs.ExecutorMember{
		MemberID: relation.String(), ParentID: parent.MemberID,
		SpawnCallID: managed.call.ID,
	}
	request, receipt := runs.NewChildRunReservationRequest()
	if err := i.sendExecutorRequest(ctx, runs.ExecutorEvent{Member: member, Payload: request}); err != nil {
		return runs.ChildRunBinding{}, err
	}
	binding, err := receipt.Await(ctx)
	if err != nil {
		return runs.ChildRunBinding{}, err
	}
	if binding.MemberID != member.MemberID || binding.ParentRunID.IsZero() {
		return runs.ChildRunBinding{}, errors.New("child Run reservation returned a different executor member")
	}
	return binding, nil
}

func sameManagedAdmission(left, right scoperun.ProcessAdmission) bool {
	return left.Valid() && right.Valid() && left.ProcessID() == right.ProcessID() &&
		left.ProgramRef() == right.ProgramRef() &&
		left.Descriptor().Digest() == right.Descriptor().Digest() &&
		left.Budget() == right.Budget() &&
		slices.Equal(left.Capabilities().Values(), right.Capabilities().Values())
}
