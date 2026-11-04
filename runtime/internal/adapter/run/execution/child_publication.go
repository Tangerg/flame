package execution

import (
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	"github.com/Tangerg/scope/core/chat"
)

func (i *interactionSession) childPublications(records []scoperun.Record) ([]runs.ChildRunPublication, error) {
	var publications []runs.ChildRunPublication
	for _, record := range records {
		started, ok := record.(scoperun.Started)
		if !ok {
			continue
		}
		relation := started.Process
		if relation.Depth() == 0 {
			continue
		}
		parent, _ := relation.Parent()
		key, _ := relation.Key()
		i.state.mu.Lock()
		managed := i.state.delegateCalls[delegateCallIdentity{parentID: parent, childKey: key}]
		i.state.mu.Unlock()
		if managed == nil {
			continue
		}
		managed.mu.Lock()
		admission, binding := managed.admission, managed.binding
		managed.mu.Unlock()
		if !admission.Valid() || admission.ProcessID() != relation || admission.ProgramRef() != started.Program {
			return nil, errors.New("execution: published child differs from its admission")
		}
		publications = append(publications, runs.ChildRunPublication{Binding: binding, Outcome: runs.ChildRunStarted})
	}
	return publications, nil
}

func (i *interactionSession) failedChildPublication(tree scoperun.Log, parent scoperun.ProcessID, sequence uint64, call chat.ToolCall) (runs.ChildRunPublication, bool, error) {
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
	for _, entry := range tree.Entries {
		if started, ok := entry.Record.(scoperun.Started); ok && started.Process == admission.ProcessID() {
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
		var processID scoperun.ProcessID
		for _, candidate := range calls {
			candidate.mu.Lock()
			if candidate.binding == publication.Binding {
				candidate.childProcessID = candidate.admission.ProcessID()
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
