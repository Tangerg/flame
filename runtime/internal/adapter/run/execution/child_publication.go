package execution

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	"github.com/Tangerg/scope/core/chat"
)

func (i *interactionSession) childPublications(ctx context.Context, tree agent.TreeSnapshot) ([]runs.ChildRunPublication, error) {
	previous := make(map[agent.ProcessID]struct{})
	head, found, err := i.executionTrees.LoadExecutionTree(ctx, i.start.SessionID, tree.RootID().String())
	if err != nil {
		return nil, err
	}
	if found {
		committed, err := decodeExecutionTree(head, tree.RootID())
		if err != nil {
			return nil, err
		}
		for _, snapshot := range committed.ProcessSnapshots() {
			previous[snapshot.Relation().ProcessID()] = struct{}{}
		}
	}
	var publications []runs.ChildRunPublication
	for _, snapshot := range tree.ProcessSnapshots() {
		relation := snapshot.Relation()
		if relation.IsRoot() {
			continue
		}
		if _, published := previous[relation.ProcessID()]; published {
			continue
		}
		parent, _ := relation.ParentID()
		key, _ := relation.ChildKey()
		i.state.mu.Lock()
		managed := i.state.delegateCalls[delegateCallIdentity{parentID: parent, childKey: key}]
		i.state.mu.Unlock()
		if managed == nil {
			continue
		}
		managed.mu.Lock()
		admission, binding := managed.admission, managed.binding
		managed.mu.Unlock()
		if !admission.Valid() || admission.Relation() != relation || admission.DeploymentRef() != snapshot.DeploymentRef() {
			return nil, errors.New("execution: published child differs from its admission")
		}
		publications = append(publications, runs.ChildRunPublication{Binding: binding, Outcome: runs.ChildRunStarted})
	}
	return publications, nil
}

func (i *interactionSession) failedChildPublication(tree agent.TreeSnapshot, parent agent.ProcessID, sequence uint64, call chat.ToolCall) (runs.ChildRunPublication, bool, error) {
	key, err := interaction.DelegateChildKey(sequence, call)
	if err != nil {
		return runs.ChildRunPublication{}, false, err
	}
	i.state.mu.Lock()
	managed := i.state.delegateCalls[delegateCallIdentity{parentID: parent, childKey: key}]
	i.state.mu.Unlock()
	if managed == nil {
		return runs.ChildRunPublication{}, false, nil
	}
	managed.mu.Lock()
	admission, binding := managed.admission, managed.binding
	managed.mu.Unlock()
	if !admission.Valid() {
		return runs.ChildRunPublication{}, false, nil
	}
	for _, snapshot := range tree.ProcessSnapshots() {
		if snapshot.Relation().ProcessID() == admission.Relation().ProcessID() {
			return runs.ChildRunPublication{}, false, nil
		}
	}
	return runs.ChildRunPublication{Binding: binding, Outcome: runs.ChildRunStartAborted}, true, nil
}

func (i *interactionSession) publishChildBindings(publications []runs.ChildRunPublication) {
	i.state.mu.Lock()
	calls := make([]*managedDelegateCall, 0, len(i.state.delegateCalls))
	for _, managed := range i.state.delegateCalls {
		calls = append(calls, managed)
	}
	i.state.mu.Unlock()
	for _, publication := range publications {
		if publication.Outcome != runs.ChildRunStarted {
			continue
		}
		var managed *managedDelegateCall
		var processID agent.ProcessID
		for _, candidate := range calls {
			candidate.mu.Lock()
			if candidate.binding == publication.Binding {
				candidate.childProcessID = candidate.admission.Relation().ProcessID()
				processID = candidate.childProcessID
				managed = candidate
			}
			candidate.mu.Unlock()
			if managed != nil {
				break
			}
		}
		if managed != nil {
			i.state.mu.Lock()
			i.state.delegateChildren[processID] = managed
			i.state.mu.Unlock()
		}
	}
}
