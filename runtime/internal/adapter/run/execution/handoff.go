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

type interactionHandoffWire struct {
	RootID    string                       `json:"rootId"`
	Processes []interactionProgressWire    `json:"processes"`
	Inputs    []interactionInputTargetWire `json:"inputs"`
}

type interactionProgressWire struct {
	ProcessID       string          `json:"processId"`
	ParentID        string          `json:"parentId,omitempty"`
	OwnerID         string          `json:"ownerId"`
	Status          scoperun.Status `json:"status"`
	PreparedEffects uint64          `json:"preparedEffects"`
}

type interactionInputTargetWire struct {
	MemberID            string         `json:"memberId"`
	ProcessID           string         `json:"processId"`
	RequestID           string         `json:"requestId"`
	WaitKey             string         `json:"waitKey"`
	ApprovalInputSchema jsontext.Value `json:"approvalInputSchema,omitempty"`
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
			OwnerID: owner.MemberID, Status: snapshot.Status, PreparedEffects: snapshot.Usage.PreparedEffects})
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
		prompt, err := runinput.DecodePrompt(input.Prompt().JSON())
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

func decodeHandoffProgress(values []interactionProgressWire) (map[scoperun.ProcessID]interactionProgressWire, map[scoperun.ProcessID]struct{}, error) {
	progress := make(map[scoperun.ProcessID]interactionProgressWire, len(values))
	processes := make(map[scoperun.ProcessID]struct{}, len(values))
	for index, value := range values {
		id, err := scoperun.ParseProcessID(value.ProcessID)
		if err != nil {
			return nil, nil, err
		}
		if index > 0 && value.ProcessID <= values[index-1].ProcessID {
			return nil, nil, errors.New("handoff progress is not canonical")
		}
		if !value.Status.Valid() {
			return nil, nil, fmt.Errorf("invalid handoff status %q", value.Status)
		}
		owner, err := scoperun.ParseProcessID(value.OwnerID)
		if err != nil || !owner.Contains(id) {
			return nil, nil, errors.New("handoff Process has an invalid product owner")
		}
		progress[id], processes[id] = value, struct{}{}
	}
	return progress, processes, nil
}

func validateHandoffTree(root scoperun.ProcessID, progress map[scoperun.ProcessID]interactionProgressWire, processes map[scoperun.ProcessID]struct{}) error {
	if _, found := processes[root]; !found {
		return errors.New("handoff root is absent")
	}
	for id, value := range progress {
		if id == root {
			if value.ParentID != "" {
				return errors.New("handoff root has a parent")
			}
			continue
		}
		parent, err := scoperun.ParseProcessID(value.ParentID)
		if err != nil {
			return err
		}
		if _, found := processes[parent]; !found {
			return errors.New("handoff parent is absent")
		}
	}
	return nil
}

func decodeInteractionHandoff(wire *interactionHandoffWire) (interactionHandoff, map[scoperun.ProcessID]struct{}, error) {
	if wire == nil || len(wire.Processes) == 0 {
		return interactionHandoff{}, nil, errors.New("handoff progress is required")
	}
	root, err := scoperun.ParseProcessID(wire.RootID)
	if err != nil {
		return interactionHandoff{}, nil, err
	}
	progress, processes, err := decodeHandoffProgress(wire.Processes)
	if err != nil {
		return interactionHandoff{}, nil, err
	}
	if err := validateHandoffTree(root, progress, processes); err != nil {
		return interactionHandoff{}, nil, err
	}
	handoff := interactionHandoff{rootID: root, progress: progress, inputs: make(map[string]interactionInputTarget)}
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
