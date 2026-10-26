package execution

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/fingerprint"
	runtimeidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
)

// toolResultMetadata retains unpublished call metadata across execution batches
// and waits, including Delegates admitted before the current child batch.
// Scope's settled child output remains the sole source of the model result.
// The checkpoint retains this data when a completed sibling waits for another
// sibling's input; no completed Tool is rerun to reconstruct its presentation.
type toolResultMetadata struct {
	Receipt      *toolReceiptEvidence `json:"receipt,omitzero"`
	MemberID     string               `json:"member_id"`
	Start        runs.ToolCallStarted `json:"start"`
	Result       *tool.Result         `json:"result,omitzero"`
	Offload      *toolresult.Ref      `json:"offload,omitzero"`
	OutputText   string               `json:"output_text,omitempty"`
	MutatedPaths []string             `json:"mutated_paths,omitempty"`
	Failure      *tool.Failure        `json:"failure,omitzero"`
}

type toolReceiptEvidence struct {
	ProcessID string             `json:"process_id"`
	EffectID  string             `json:"effect_id"`
	Tool      tool.Ref           `json:"tool"`
	Release   fingerprint.Digest `json:"release"`
	Authority fingerprint.Digest `json:"authority"`
}

func (t toolResultMetadata) clone() toolResultMetadata {
	if t.Receipt != nil {
		t.Receipt = new(*t.Receipt)
	}
	t.MutatedPaths = slices.Clone(t.MutatedPaths)
	if t.Result != nil {
		t.Result = new(*t.Result)
	}
	if t.Offload != nil {
		t.Offload = new(*t.Offload)
	}
	if t.Failure != nil {
		t.Failure = new(*t.Failure)
	}
	return t
}

func (t toolResultMetadata) validate() error {
	if t.Receipt != nil {
		if _, err := agent.ParseProcessID(t.Receipt.ProcessID); err != nil {
			return err
		}
		if _, err := agent.ParseEffectID(t.Receipt.EffectID); err != nil {
			return err
		}
		server, _, found := t.Receipt.Tool.MCP()
		_, installed := server.Origin().Installation()
		if !found || !installed || t.Receipt.Tool.ModelName() != t.Start.ToolName {
			return errors.New("execution: invalid Tool receipt evidence")
		}
		if err := t.Receipt.Release.Validate(); err != nil {
			return err
		}
		if err := t.Receipt.Authority.Validate(); err != nil {
			return err
		}
	}
	processID, err := agent.ParseProcessID(t.MemberID)
	if err != nil || t.Start.ModelCallSequence == 0 || t.Start.SourceCallID == "" || t.Start.ToolName == "" {
		return errors.New("execution: invalid Tool metadata attribution")
	}
	identity, err := logicalToolCallID(processID, t.Start.ModelCallSequence, t.Start.ToolCallIndex, t.Start.SourceCallID, t.Start.ToolName)
	if err != nil || identity.String() != t.Start.CallID {
		return errors.New("execution: Tool metadata logical identity changed")
	}
	if _, err := tool.ParseArguments(t.Start.Arguments); err != nil {
		return err
	}
	if t.Offload != nil {
		if err := t.Offload.Validate(); err != nil {
			return err
		}
		if t.Result == nil {
			return errors.New("execution: offloaded Tool metadata has no preview")
		}
		if _, text := t.Result.String(); !text {
			return errors.New("execution: offloaded Tool preview must be text")
		}
	}
	if t.Failure != nil {
		return t.Failure.Validate()
	}
	return nil
}

func (i *interactionSession) rememberToolMetadata(metadata toolResultMetadata) error {
	if err := metadata.validate(); err != nil {
		return err
	}
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.toolMetadata == nil {
		i.state.toolMetadata = make(map[string]toolResultMetadata)
	}
	if previous, duplicate := i.state.toolMetadata[metadata.Start.CallID]; duplicate {
		if previous.Receipt == nil || previous.Result != nil || previous.Failure != nil || previous.Offload != nil || previous.Start != metadata.Start {
			return errors.New("execution: pending call metadata already exists")
		}
		metadata.Receipt = new(*previous.Receipt)
	}
	i.state.toolMetadata[metadata.Start.CallID] = metadata.clone()
	return nil
}

func (i *interactionSession) toolResultProjection(relation agent.ProcessRelation, sequence uint64, entry interaction.ResultEntry) (runs.ExecutorEvent, error) {
	member := i.executorMember(relation)
	identity, err := logicalToolCallID(relation.ProcessID(), sequence, entry.ToolCallIndex, entry.Call.ID, entry.Call.Name)
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
		if metadata.MemberID != member.MemberID {
			return runs.ExecutorEvent{}, errors.New("execution: result metadata belongs to another member")
		}
		start = metadata.Start
		end.Arguments, end.Result, end.Offload = metadata.Start.Arguments, metadata.Result, metadata.Offload
		end.OutputText, end.MutatedPaths, end.Failure = metadata.OutputText, metadata.MutatedPaths, metadata.Failure
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

func (i *interactionSession) retirePublishedToolMetadata(identities []runtimeidentity.EffectID) {
	i.state.mu.Lock()
	var delegates []*managedDelegateCall
	for _, identity := range identities {
		if i.state.toolMetadata[identity.String()].Receipt == nil {
			delete(i.state.toolMetadata, identity.String())
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

func decodeToolMetadata(values []toolResultMetadata, processes map[agent.ProcessID]struct{}) (map[string]toolResultMetadata, error) {
	metadata := make(map[string]toolResultMetadata, len(values))
	previous := ""
	for _, value := range values {
		if err := value.validate(); err != nil {
			return nil, err
		}
		processID, _ := agent.ParseProcessID(value.MemberID)
		if _, found := processes[processID]; !found || value.Start.CallID <= previous {
			return nil, errors.New("execution: noncanonical or foreign Tool metadata")
		}
		previous = value.Start.CallID
		metadata[previous] = value.clone()
	}
	return metadata, nil
}

func resultPublication(identity runtimeidentity.EffectID, entry interaction.ResultEntry) (runs.ResultPublication, error) {
	data, err := json.Marshal(entry)
	if err != nil {
		return runs.ResultPublication{}, err
	}
	return runs.ResultPublication{ID: identity.String(), Digest: agent.ComputeDigest(data).String()}, nil
}
