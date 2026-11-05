package execution

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"slices"
	"strings"

	runinput "github.com/Tangerg/flame/runtime/internal/adapter/run/input"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	scoperun "github.com/Tangerg/scope/agent/run"
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
	ProcessID       string          `json:"process_id"`
	ParentID        string          `json:"parent_id,omitempty"`
	OwnerID         string          `json:"owner_id"`
	Status          scoperun.Status `json:"status"`
	CommittedSteps  uint64          `json:"committed_steps"`
	PreparedEffects uint64          `json:"prepared_effects"`
}

type interactionInputTargetWire struct {
	MemberID            string         `json:"member_id"`
	ProcessID           string         `json:"process_id"`
	RequestID           string         `json:"request_id"`
	WaitKey             string         `json:"wait_key"`
	ApprovalInputSchema jsontext.Value `json:"approval_input_schema,omitempty"`
}

type interactionInputTarget struct {
	memberID            scoperun.ProcessID
	waitKey             scoperun.WaitKey
	processID           scoperun.ProcessID
	approvalInputSchema jsonschema.Schema
}

type interactionHandoff struct {
	rootID   scoperun.ProcessID
	progress map[scoperun.ProcessID]interactionProgressWire
	inputs   map[string]interactionInputTarget
}

func (i *interactionSession) encodeInteractionHandoff(tree scoperun.Log) (*interactionHandoffWire, error) {
	inspection, err := tree.Inspect()
	if err != nil {
		return nil, err
	}
	wire := &interactionHandoffWire{RootID: tree.Root.String()}
	for _, snapshot := range inspection.Processes {
		parent, _ := snapshot.ID.Parent()
		owner := i.executorMember(snapshot.ID)
		if owner.MemberID == "" {
			return nil, errors.New("execution: handoff Process has no product owner")
		}
		wire.Processes = append(wire.Processes, interactionProgressWire{ProcessID: snapshot.ID.String(), ParentID: parent.String(),
			OwnerID: owner.MemberID, Status: snapshot.Status, CommittedSteps: snapshot.Usage.CommittedSteps, PreparedEffects: snapshot.Usage.PreparedEffects})
	}
	slices.SortFunc(wire.Processes, func(a, b interactionProgressWire) int { return strings.Compare(a.ProcessID, b.ProcessID) })
	inputs, err := i.pendingInputs(tree)
	if err != nil {
		return nil, err
	}
	for _, input := range inputs {
		target, found := inspection.Process(input.ProcessID())
		member, owned := i.toolCallMember(input.ProcessID())
		if !found || !owned || target.Status != scoperun.StatusPaused {
			continue
		}
		ownerID, err := scoperun.ParseProcessID(member.MemberID)
		if err != nil {
			return nil, err
		}
		owner, found := inspection.Process(ownerID)
		if !found || owner.Status != scoperun.StatusPaused {
			continue
		}
		value := interactionInputTargetWire{ProcessID: input.ProcessID().String(), MemberID: member.MemberID,
			RequestID: inputRequestID(input.ProcessID(), input.WaitKey()), WaitKey: input.WaitKey().String()}
		prompt, err := runinput.DecodePrompt(input.Prompt())
		if err != nil {
			return nil, err
		}
		if approval, ok := prompt.(runs.ApprovalPrompt); ok {
			contract, found := i.state.deployments.toolContracts[target.Program][approval.Tool]
			if !found {
				return nil, errors.New("execution: approval Tool contract is unavailable")
			}
			value.ApprovalInputSchema = contract.Definition().InputSchema
		}
		wire.Inputs = append(wire.Inputs, value)
	}
	slices.SortFunc(wire.Inputs, func(a, b interactionInputTargetWire) int { return strings.Compare(a.RequestID, b.RequestID) })
	return wire, nil
}

func decodeInteractionHandoff(wire *interactionHandoffWire) (interactionHandoff, map[scoperun.ProcessID]struct{}, error) {
	if wire == nil || len(wire.Processes) == 0 {
		return interactionHandoff{}, nil, errors.New("handoff progress is required")
	}
	root, err := scoperun.ParseProcessID(wire.RootID)
	if err != nil {
		return interactionHandoff{}, nil, err
	}
	handoff := interactionHandoff{rootID: root, progress: make(map[scoperun.ProcessID]interactionProgressWire), inputs: make(map[string]interactionInputTarget)}
	processes := make(map[scoperun.ProcessID]struct{})
	for index, value := range wire.Processes {
		id, err := scoperun.ParseProcessID(value.ProcessID)
		if err != nil {
			return interactionHandoff{}, nil, err
		}
		if index > 0 && value.ProcessID <= wire.Processes[index-1].ProcessID {
			return interactionHandoff{}, nil, errors.New("handoff progress is not canonical")
		}
		if !value.Status.Valid() {
			return interactionHandoff{}, nil, fmt.Errorf("invalid handoff status %q", value.Status)
		}
		owner, err := scoperun.ParseProcessID(value.OwnerID)
		if err != nil || !owner.Contains(id) {
			return interactionHandoff{}, nil, errors.New("handoff Process has an invalid product owner")
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
		parent, err := scoperun.ParseProcessID(progress.ParentID)
		if err != nil {
			return interactionHandoff{}, nil, err
		}
		if _, found := processes[parent]; !found {
			return interactionHandoff{}, nil, errors.New("handoff parent is absent")
		}
	}
	if err := handoff.decodeInputs(wire.Inputs); err != nil {
		return interactionHandoff{}, nil, err
	}
	return handoff, processes, nil
}

func (handoff *interactionHandoff) decodeInputs(inputs []interactionInputTargetWire) error {
	for index, value := range inputs {
		waitID, err := scoperun.ParseWaitKey(value.WaitKey)
		if err != nil {
			return err
		}
		member, err := scoperun.ParseProcessID(value.MemberID)
		if err != nil {
			return err
		}
		process, err := scoperun.ParseProcessID(value.ProcessID)
		if err != nil {
			return err
		}
		if index > 0 && value.RequestID <= inputs[index-1].RequestID {
			return errors.New("handoff inputs are not canonical")
		}
		if handoff.progress[member].Status != scoperun.StatusPaused || handoff.progress[process].Status != scoperun.StatusPaused || handoff.progress[process].OwnerID != member.String() {
			return errors.New("handoff input target is outside the paused members")
		}
		var schema jsonschema.Schema
		if len(value.ApprovalInputSchema) > 0 {
			schema, err = jsonschema.Parse(value.ApprovalInputSchema)
			if err != nil {
				return err
			}
		}
		if value.RequestID != inputRequestID(process, waitID) {
			return errors.New("handoff input identity differs from its process and wait")
		}
		handoff.inputs[value.RequestID] = interactionInputTarget{memberID: member, processID: process, waitKey: waitID, approvalInputSchema: schema}
	}
	return nil
}
