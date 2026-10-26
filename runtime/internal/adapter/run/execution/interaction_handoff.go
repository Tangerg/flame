package execution

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"slices"
	"strings"

	runinput "github.com/Tangerg/flame/runtime/internal/adapter/run/input"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	"github.com/Tangerg/scope/core/jsonschema"
)

// These immutable Scope projections identify the original handoff. They cannot
// restore a tree: only the acknowledged execution-tree head carries that state.
type interactionHandoffWire struct {
	RootID    string                       `json:"root_id"`
	Processes []interactionProgressWire    `json:"processes"`
	Inputs    []interactionInputTargetWire `json:"inputs"`
}

type interactionProgressWire struct {
	ProcessID       string       `json:"process_id"`
	ParentID        string       `json:"parent_id,omitempty"`
	Status          agent.Status `json:"status"`
	CommittedSteps  uint64       `json:"committed_steps"`
	PreparedEffects uint64       `json:"prepared_effects"`
}

type interactionInputTargetWire struct {
	MemberID            string         `json:"member_id"`
	ProcessID           string         `json:"process_id"`
	WaitID              string         `json:"wait_id"`
	ApprovalInputSchema jsontext.Value `json:"approval_input_schema,omitempty"`
}

type interactionInputTarget struct {
	memberID            agent.ProcessID
	processID           agent.ProcessID
	approvalInputSchema jsonschema.Schema
}

type interactionHandoff struct {
	rootID   agent.ProcessID
	progress map[agent.ProcessID]interactionProgressWire
	inputs   map[agent.WaitID]interactionInputTarget
}

func (i *interactionSession) encodeInteractionHandoff(tree agent.TreeSnapshot) (*interactionHandoffWire, error) {
	wire := &interactionHandoffWire{RootID: tree.RootID().String()}
	snapshots := make(map[agent.ProcessID]agent.ProcessSnapshot)
	for _, snapshot := range tree.ProcessSnapshots() {
		id := snapshot.Relation().ProcessID()
		snapshots[id] = snapshot
		parentID, _ := snapshot.Relation().ParentID()
		parent := ""
		if parentID.Valid() {
			parent = parentID.String()
		}
		wire.Processes = append(wire.Processes, interactionProgressWire{ProcessID: id.String(), ParentID: parent, Status: snapshot.Status(), CommittedSteps: snapshot.Usage().CommittedSteps, PreparedEffects: snapshot.Usage().PreparedEffects})
	}
	slices.SortFunc(wire.Processes, func(a, b interactionProgressWire) int { return strings.Compare(a.ProcessID, b.ProcessID) })
	inputs, err := interaction.PendingToolInputs(tree)
	if err != nil {
		return nil, err
	}
	for _, input := range inputs {
		target := snapshots[input.ProcessID()]
		parent, child := target.Relation().ParentID()
		if !child {
			return nil, errors.New("execution: input target has no product member")
		}
		if target.Status() != agent.StatusPaused || snapshots[parent].Status() != agent.StatusPaused {
			continue
		}
		value := interactionInputTargetWire{ProcessID: input.ProcessID().String(), MemberID: parent.String(), WaitID: input.WaitID().String()}
		prompt, err := runinput.DecodePrompt(input.Prompt())
		if err != nil {
			return nil, err
		}
		if prompt.Kind == run.InterruptApproval {
			i.state.mu.Lock()
			contract, found := i.state.deployments.toolContracts[target.DeploymentRef()][prompt.Approval.Tool]
			i.state.mu.Unlock()
			if !found {
				return nil, errors.New("execution: approval Tool contract is unavailable")
			}
			value.ApprovalInputSchema = contract.Definition().InputSchema
		}
		wire.Inputs = append(wire.Inputs, value)
	}
	slices.SortFunc(wire.Inputs, func(a, b interactionInputTargetWire) int { return strings.Compare(a.WaitID, b.WaitID) })
	return wire, nil
}

func decodeInteractionHandoff(wire *interactionHandoffWire) (interactionHandoff, map[agent.ProcessID]struct{}, error) {
	if wire == nil || len(wire.Processes) == 0 {
		return interactionHandoff{}, nil, errors.New("handoff progress is required")
	}
	root, err := agent.ParseProcessID(wire.RootID)
	if err != nil {
		return interactionHandoff{}, nil, err
	}
	handoff := interactionHandoff{rootID: root, progress: make(map[agent.ProcessID]interactionProgressWire), inputs: make(map[agent.WaitID]interactionInputTarget)}
	processes := make(map[agent.ProcessID]struct{})
	for index, value := range wire.Processes {
		id, err := agent.ParseProcessID(value.ProcessID)
		if err != nil {
			return interactionHandoff{}, nil, err
		}
		if index > 0 && value.ProcessID <= wire.Processes[index-1].ProcessID {
			return interactionHandoff{}, nil, errors.New("handoff progress is not canonical")
		}
		switch value.Status {
		case agent.StatusRunning, agent.StatusWaiting, agent.StatusPaused, agent.StatusCompleted, agent.StatusFailed, agent.StatusCanceled:
		default:
			return interactionHandoff{}, nil, fmt.Errorf("invalid handoff status %q", value.Status)
		}
		handoff.progress[id], processes[id] = value, struct{}{}
	}
	if _, found := processes[root]; !found {
		return interactionHandoff{}, nil, errors.New("handoff root is absent")
	}
	for id, progress := range handoff.progress {
		if id == root {
			if progress.ParentID != "" {
				return interactionHandoff{}, nil, errors.New("handoff root has a parent")
			}
			continue
		}
		parent, err := agent.ParseProcessID(progress.ParentID)
		if err != nil {
			return interactionHandoff{}, nil, err
		}
		if _, found := processes[parent]; !found {
			return interactionHandoff{}, nil, errors.New("handoff parent is absent")
		}
	}
	for index, value := range wire.Inputs {
		waitID, err := agent.ParseWaitID(value.WaitID)
		if err != nil {
			return interactionHandoff{}, nil, err
		}
		member, err := agent.ParseProcessID(value.MemberID)
		if err != nil {
			return interactionHandoff{}, nil, err
		}
		process, err := agent.ParseProcessID(value.ProcessID)
		if err != nil {
			return interactionHandoff{}, nil, err
		}
		if index > 0 && value.WaitID <= wire.Inputs[index-1].WaitID {
			return interactionHandoff{}, nil, errors.New("handoff inputs are not canonical")
		}
		if handoff.progress[member].Status != agent.StatusPaused || handoff.progress[process].Status != agent.StatusPaused || member == process {
			return interactionHandoff{}, nil, errors.New("handoff input target is outside the paused members")
		}
		var schema jsonschema.Schema
		if len(value.ApprovalInputSchema) > 0 {
			schema, err = jsonschema.Parse(value.ApprovalInputSchema)
			if err != nil {
				return interactionHandoff{}, nil, err
			}
		}
		handoff.inputs[waitID] = interactionInputTarget{memberID: member, processID: process, approvalInputSchema: schema}
	}
	return handoff, processes, nil
}
