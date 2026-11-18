package execution

import (
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"

	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
)

// A checkpoint retains call attribution and receipt authority across waits.
// A completed outcome is durable only with its Scope settlement and Item.
type toolCallCapture struct {
	Receipt   *toolReceiptEvidence `json:"receipt,omitzero"`
	MemberID  string               `json:"member_id"`
	ProcessID scoperun.ProcessID   `json:"process_id"`
	Start     runs.ToolCallStarted `json:"start"`
}

type toolCallMetadata struct {
	toolCallCapture
	presentation *toolResultPresentation
}

type toolResultPresentation struct {
	result       *tool.Result
	offload      *transcript.ToolResultOffload
	outputText   string
	mutatedPaths []string
	failure      *tool.Failure
}

type toolReceiptEvidence struct {
	EffectID  scoperun.EffectID `json:"effect_id"`
	Release   framevalue.Digest `json:"release"`
	Authority framevalue.Digest `json:"authority"`
}

func (r toolReceiptEvidence) validate(processID scoperun.ProcessID, source tool.Ref) error {
	if !r.EffectID.Valid() || r.EffectID.Process != processID {
		return errors.New("execution: Tool receipt Effect differs from its call")
	}
	server, _, found := source.MCP()
	_, installed := server.Origin().Installation()
	if !found || !installed {
		return errors.New("execution: invalid Tool receipt evidence")
	}
	if err := r.Release.Validate(); err != nil {
		return err
	}
	return r.Authority.Validate()
}

func (t toolCallCapture) validate() error {
	if err := runs.ValidateMember(t.MemberID); err != nil {
		return err
	}
	if t.Start.Source != (tool.Ref{}) && !t.Start.Source.MatchesPublishedName(t.Start.ToolName) {
		return errors.New("execution: captured tool name changed")
	}
	if !t.ProcessID.Valid() || t.Start.ModelCallSequence == 0 || t.Start.SourceCallID == "" || t.Start.ToolName == "" {
		return errors.New("execution: invalid Tool metadata attribution")
	}
	identity, err := logicalToolCallID(t.ProcessID, t.Start.ModelCallSequence, t.Start.ToolCallIndex, t.Start.SourceCallID, t.Start.ToolName)
	if err != nil || identity.String() != t.Start.CallID {
		return errors.New("execution: Tool metadata logical identity changed")
	}
	if t.Receipt != nil {
		if err := t.Receipt.validate(t.ProcessID, t.Start.Source); err != nil {
			return err
		}
	}
	_, err = tool.ParseArguments(t.Start.Arguments)
	return err
}

func (i *interactionSession) rememberToolMetadata(metadata toolCallMetadata) error {
	if err := metadata.validate(); err != nil {
		return err
	}
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.toolMetadata == nil {
		i.state.toolMetadata = make(map[string]toolCallMetadata)
	}
	if previous, duplicate := i.state.toolMetadata[metadata.Start.CallID]; duplicate {
		if previous.Receipt == nil || previous.presentation != nil || metadata.presentation == nil ||
			previous.Start != metadata.Start || previous.MemberID != metadata.MemberID || previous.ProcessID != metadata.ProcessID {
			return errors.New("execution: pending call metadata already exists")
		}
		metadata.toolCallCapture = previous.toolCallCapture
	}
	i.state.toolMetadata[metadata.Start.CallID] = metadata
	return nil
}

func (i *interactionSession) toolResultProjection(relation scoperun.ProcessID, sequence uint64, entry interaction.ResultEntry) (runs.ExecutorEvent, error) {
	member := i.executorMember(relation)
	identity, err := logicalToolCallID(relation, sequence, entry.ToolCallIndex, entry.Call.ID, entry.Call.Name)
	if err != nil {
		return runs.ExecutorEvent{}, err
	}
	publication, err := resultPublication(identity, entry)
	if err != nil {
		return runs.ExecutorEvent{}, err
	}
	fact := runs.ToolResultsCommitted{Publication: publication}
	i.state.mu.Lock()
	metadata, prepared := i.state.toolMetadata[identity.String()]
	i.state.mu.Unlock()
	start := runs.ToolCallStarted{
		CallID: identity.String(), SourceCallID: entry.Call.ID, ModelCallSequence: sequence,
		ToolCallIndex: entry.ToolCallIndex, ToolName: entry.Call.Name, ArgumentsText: entry.Call.Arguments,
	}
	end := runs.ToolCallFinished{CallID: identity.String(), ModelResult: new(entry.Result.Clone())}
	if prepared {
		if metadata.MemberID != member.MemberID || metadata.ProcessID != relation {
			return runs.ExecutorEvent{}, errors.New("execution: result metadata belongs to another member")
		}
		start = metadata.Start
		end.Arguments = metadata.Start.Arguments
		if presentation := metadata.presentation; presentation != nil {
			end.Result, end.Offload = presentation.result, presentation.offload
			end.OutputText, end.MutatedPaths, end.Failure = presentation.outputText, presentation.mutatedPaths, presentation.failure
		} else if metadata.Receipt != nil {
			return runs.ExecutorEvent{}, errors.New("execution: captured Tool settled without its presentation")
		}
	} else if entry.Disposition != interaction.ResultRejected {
		return runs.ExecutorEvent{}, fmt.Errorf("execution: known Tool result %q lost its product metadata", entry.Call.ID)
	}
	if end.Result == nil {
		result, present, err := runtimeToolResult(entry.Result.Output)
		if err != nil {
			return runs.ExecutorEvent{}, err
		}
		if present {
			end.Result = &result
		}
	}
	if entry.Result.IsError && end.Failure == nil {
		detail, _ := entry.Result.Output.Text()
		end.Failure = &tool.Failure{Kind: tool.FailureExecution, Detail: detail}
	}
	fact.Starts = append(fact.Starts, start)
	fact.Results = append(fact.Results, end)
	return runs.ExecutorEvent{Member: member, Payload: fact}, nil
}

func (i *interactionSession) retirePublishedToolMetadata(identities []runs.InvocationID) {
	i.state.mu.Lock()
	var delegates []*managedDelegateCall
	for _, identity := range identities {
		id := identity.String()
		if metadata := i.state.toolMetadata[id]; metadata.Receipt != nil {
			metadata.presentation = nil
			i.state.toolMetadata[id] = metadata
		} else {
			delete(i.state.toolMetadata, id)
		}
		for _, managed := range i.state.delegateCalls {
			if managed.callID == identity {
				delegates = append(delegates, managed)
				break
			}
		}
	}
	i.state.mu.Unlock()
	for _, managed := range delegates {
		managed.mu.Lock()
		managed.parentToolFinished = true
		managed.mu.Unlock()
	}
}

// The next model dispatch closes this member's investigated Tool prefix.
// Historical Tool metadata cannot turn a later model crash into that class.
func (i *interactionSession) retireToolReceiptPrefix(memberID string) {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	for id, metadata := range i.state.toolMetadata {
		if metadata.MemberID == memberID && metadata.Receipt != nil {
			delete(i.state.toolMetadata, id)
		}
	}
}

func decodeToolCalls(values []toolCallCapture, processes map[scoperun.ProcessID]struct{}) (map[string]toolCallMetadata, error) {
	metadata := make(map[string]toolCallMetadata, len(values))
	previous := ""
	for _, value := range values {
		if err := value.validate(); err != nil {
			return nil, err
		}
		processID, _ := scoperun.ParseProcessID(value.MemberID)
		if _, found := processes[processID]; !found || value.Start.CallID <= previous {
			return nil, errors.New("execution: noncanonical or foreign Tool metadata")
		}
		if _, found := processes[value.ProcessID]; !found {
			return nil, errors.New("execution: Tool metadata source is outside the execution tree")
		}
		previous = value.Start.CallID
		metadata[previous] = toolCallMetadata{toolCallCapture: value}
	}
	return metadata, nil
}

func resultPublication(identity runs.InvocationID, entry interaction.ResultEntry) (runs.ResultPublication, error) {
	// A locally rejected proposal has no Scope Effect. Its product receipt still
	// binds the exact model-visible result to the original call coordinates.
	data, err := json.Marshal(struct {
		Reference   string                        `json:"reference,omitempty"`
		Index       uint32                        `json:"index"`
		Call        corechat.ToolCall             `json:"call"`
		Result      corechat.ToolResult           `json:"result"`
		Disposition interaction.ResultDisposition `json:"disposition"`
	}{entry.Reference.String(), entry.ToolCallIndex, entry.Call, entry.Result, entry.Disposition}, json.Deterministic(true))
	if err != nil {
		return runs.ResultPublication{}, err
	}
	return runs.ResultPublication{ID: identity.String(), Digest: scoperun.ComputeDigest(data).String()}, nil
}
