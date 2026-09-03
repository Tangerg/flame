package agentexec

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/strictjson"
	agent "github.com/Tangerg/scope/agent"
	corechat "github.com/Tangerg/scope/core/chat"
)

const interactionCheckpointSchemaVersion uint16 = 6

type interactionCheckpointPayloadWire struct {
	SchemaVersion       uint16                              `json:"schema_version"`
	Tree                json.RawMessage                     `json:"tree"`
	Instructions        []corechat.Message                  `json:"instructions,omitempty"`
	Members             []interactionMemberCallsWire        `json:"members,omitempty"`
	Carried             []interactionModelCallsWire         `json:"carried,omitempty"`
	Contexts            []interactionModelContextWire       `json:"contexts,omitempty"`
	PendingSteers       []interactionPendingSteerWire       `json:"pending_steers,omitempty"`
	PendingContinuation *interactionPendingContinuationWire `json:"pending_continuation,omitempty"`
}

type interactionMemberCallsWire struct {
	MemberID string                      `json:"member_id"`
	Models   []interactionModelCallsWire `json:"models"`
}

type interactionModelCallsWire struct {
	Model string `json:"model"`
	Calls int    `json:"calls"`
}

type interactionModelContextWire struct {
	MemberID        string `json:"member_id"`
	ReportedTokens  int64  `json:"reported_tokens"`
	EstimatedTokens int    `json:"estimated_tokens"`
}

type interactionPendingSteerWire struct {
	SignalID string                        `json:"signal_id"`
	Content  []interactionContentBlockWire `json:"content"`
}

type interactionPendingContinuationWire struct {
	MemberID string                        `json:"member_id"`
	ItemID   string                        `json:"item_id"`
	Content  []interactionContentBlockWire `json:"content"`
}

type interactionContentBlockWire struct {
	Kind      transcript.ContentKind `json:"kind"`
	Text      string                 `json:"text,omitempty"`
	MediaType string                 `json:"media_type,omitempty"`
	Data      string                 `json:"data,omitempty"`
}

type interactionCheckpointState struct {
	tree                agent.TreeSnapshot
	callsByProcess      map[agent.ProcessID]map[string]int
	carriedCallCount    map[string]int
	contextByProcess    map[agent.ProcessID]ModelContextTokenCalibration
	instructions        []corechat.Message
	pendingSteers       map[agent.SignalID]pendingInteractionSteer
	pendingContinuation *pendingInteractionContinuation
}

func (i *interactionSession) executorCheckpoint(
	tree agent.TreeSnapshot,
) (runs.ExecutorCheckpoint, error) {
	payload, err := i.interactionCheckpointPayload(tree)
	if err != nil {
		return runs.ExecutorCheckpoint{}, err
	}
	usage, err := i.accounting.snapshot()
	if err != nil {
		return runs.ExecutorCheckpoint{}, err
	}
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: tree.RootID().String(), Payload: payload,
		BuildID: i.buildID.String(), Scope: i.scope,
		ModelSelection: i.start.ModelSelection, Limits: i.start.Limits,
		Capabilities: run.Capabilities{
			ChildRuns:      i.start.ChildRunAdmissionEnabled,
			InterruptKinds: slices.Clone(i.start.InterruptKinds),
		},
		Usage: usage,
	}
	if err := checkpoint.Validate(); err != nil {
		return runs.ExecutorCheckpoint{}, err
	}
	return checkpoint, nil
}

func encodeInteractionCheckpointPayload(
	tree agent.TreeSnapshot,
	usageByProcess map[agent.ProcessID]map[string]accounting.ModelUsage,
	carriedUsage map[string]accounting.ModelUsage,
	contextByProcess map[agent.ProcessID]ModelContextTokenCalibration,
	instructions []corechat.Message,
	pendingSteers map[agent.SignalID]pendingInteractionSteer,
	pendingContinuation *pendingInteractionContinuation,
) ([]byte, error) {
	if !tree.Valid() {
		return nil, errors.New("agentexec: encode invalid Interaction tree checkpoint")
	}
	wire := interactionCheckpointPayloadWire{
		SchemaVersion: interactionCheckpointSchemaVersion,
		Tree:          tree.JSON(),
		Instructions:  cloneChatMessages(instructions),
	}
	if _, err := interactionInstructionContext(wire.Instructions); err != nil {
		return nil, err
	}
	for processID, byModel := range usageByProcess {
		models, err := interactionCallCounts(byModel)
		if err != nil {
			return nil, fmt.Errorf("agentexec: encode Interaction member %s accounting: %w", processID, err)
		}
		if len(models) == 0 {
			continue
		}
		wire.Members = append(wire.Members, interactionMemberCallsWire{
			MemberID: processID.String(), Models: models,
		})
	}
	slices.SortFunc(wire.Members, func(left, right interactionMemberCallsWire) int {
		return strings.Compare(left.MemberID, right.MemberID)
	})
	var err error
	wire.Carried, err = interactionCallCounts(carriedUsage)
	if err != nil {
		return nil, fmt.Errorf("agentexec: encode carried Interaction accounting: %w", err)
	}
	wire.Contexts, err = encodeInteractionModelContexts(contextByProcess)
	if err != nil {
		return nil, fmt.Errorf("agentexec: encode Interaction model contexts: %w", err)
	}
	wire.PendingSteers, err = encodeInteractionPendingSteers(pendingSteers)
	if err != nil {
		return nil, fmt.Errorf("agentexec: encode pending Interaction steers: %w", err)
	}
	wire.PendingContinuation, err = encodeInteractionPendingContinuation(
		pendingContinuation, tree.RootID(),
	)
	if err != nil {
		return nil, fmt.Errorf("agentexec: encode pending Interaction continuation: %w", err)
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("agentexec: encode Interaction checkpoint: %w", err)
	}
	return payload, nil
}

func encodeInteractionModelContexts(
	contexts map[agent.ProcessID]ModelContextTokenCalibration,
) ([]interactionModelContextWire, error) {
	values := make([]interactionModelContextWire, 0, len(contexts))
	for processID, calibration := range contexts {
		if !processID.Valid() {
			return nil, errors.New("model context calibration has an invalid Process identity")
		}
		if err := calibration.Validate(); err != nil {
			return nil, err
		}
		if calibration == (ModelContextTokenCalibration{}) {
			continue
		}
		values = append(values, interactionModelContextWire{
			MemberID:        processID.String(),
			ReportedTokens:  calibration.ReportedTokens(),
			EstimatedTokens: calibration.EstimatedTokens(),
		})
	}
	slices.SortFunc(values, func(left, right interactionModelContextWire) int {
		return strings.Compare(left.MemberID, right.MemberID)
	})
	return values, nil
}

func encodeInteractionPendingSteers(
	pending map[agent.SignalID]pendingInteractionSteer,
) ([]interactionPendingSteerWire, error) {
	values := make([]interactionPendingSteerWire, 0, len(pending))
	for signalID, steer := range pending {
		value, err := encodeInteractionPendingSteer(signalID, steer)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	slices.SortFunc(values, func(left, right interactionPendingSteerWire) int {
		return strings.Compare(left.SignalID, right.SignalID)
	})
	return values, nil
}

func encodeInteractionPendingSteer(
	signalID agent.SignalID,
	steer pendingInteractionSteer,
) (interactionPendingSteerWire, error) {
	if !signalID.Valid() {
		return interactionPendingSteerWire{}, errors.New("pending steer has an invalid Signal identity")
	}
	if len(steer.content) == 0 {
		return interactionPendingSteerWire{}, fmt.Errorf("pending steer %s has no product content", signalID)
	}
	content := make([]interactionContentBlockWire, len(steer.content))
	for index, block := range steer.content {
		value, err := encodeInteractionContentBlock(block)
		if err != nil {
			return interactionPendingSteerWire{}, fmt.Errorf("pending steer %s content %d: %w", signalID, index, err)
		}
		content[index] = value
	}
	return interactionPendingSteerWire{
		SignalID: signalID.String(), Content: content,
	}, nil
}

func encodeInteractionPendingContinuation(
	pending *pendingInteractionContinuation,
	rootID agent.ProcessID,
) (*interactionPendingContinuationWire, error) {
	if pending == nil {
		return nil, nil
	}
	if !pending.processID.Valid() || pending.processID != rootID {
		return nil, errors.New("pending continuation does not name the root member")
	}
	if _, err := resourceid.ParseItem(pending.itemID); err != nil {
		return nil, fmt.Errorf("pending continuation Item: %w", err)
	}
	if len(pending.content) == 0 {
		return nil, errors.New("pending continuation has no product content")
	}
	content := make([]interactionContentBlockWire, len(pending.content))
	for index, block := range pending.content {
		value, err := encodeInteractionContentBlock(block)
		if err != nil {
			return nil, fmt.Errorf("pending continuation content %d: %w", index, err)
		}
		content[index] = value
	}
	return &interactionPendingContinuationWire{
		MemberID: pending.processID.String(), ItemID: pending.itemID, Content: content,
	}, nil
}

func encodeInteractionContentBlock(
	block transcript.ContentBlock,
) (interactionContentBlockWire, error) {
	if err := block.Validate(); err != nil {
		return interactionContentBlockWire{}, err
	}
	switch block.Kind {
	case transcript.TextContent:
		return interactionContentBlockWire{Kind: block.Kind, Text: block.Text}, nil
	case transcript.ImageContent:
		return interactionContentBlockWire{
			Kind: block.Kind, MediaType: block.MediaType,
			Data: base64.StdEncoding.EncodeToString(block.Bytes),
		}, nil
	default:
		return interactionContentBlockWire{}, fmt.Errorf("has unknown kind %q", block.Kind)
	}
}

func interactionCallCounts(
	byModel map[string]accounting.ModelUsage,
) ([]interactionModelCallsWire, error) {
	models := make([]interactionModelCallsWire, 0, len(byModel))
	for model, usage := range byModel {
		if _, err := modelref.NewModelIdentity(model); err != nil {
			return nil, err
		}
		if err := usage.Validate(); err != nil || usage.Model != model {
			if err == nil {
				err = errors.New("model key differs from usage identity")
			}
			return nil, err
		}
		if usage.Calls == 0 {
			continue
		}
		models = append(models, interactionModelCallsWire{Model: model, Calls: usage.Calls})
	}
	slices.SortFunc(models, func(left, right interactionModelCallsWire) int {
		return strings.Compare(left.Model, right.Model)
	})
	return models, nil
}

func decodeInteractionCheckpointPayload(payload []byte) (interactionCheckpointState, error) {
	wire, err := decodeInteractionCheckpointWire(payload)
	if err != nil {
		return interactionCheckpointState{}, err
	}
	tree, processes, err := decodeInteractionCheckpointTree(wire.Tree)
	if err != nil {
		return interactionCheckpointState{}, err
	}
	instructions, err := decodeInteractionCheckpointInstructions(wire.Instructions)
	if err != nil {
		return interactionCheckpointState{}, err
	}
	callsByProcess, err := decodeInteractionCheckpointMembers(wire.Members, processes)
	if err != nil {
		return interactionCheckpointState{}, err
	}
	carriedCallCount, err := decodeInteractionCallCounts(wire.Carried)
	if err != nil {
		return interactionCheckpointState{}, fmt.Errorf("agentexec: Interaction checkpoint carried calls: %w", err)
	}
	contextByProcess, err := decodeInteractionModelContexts(wire.Contexts, processes, callsByProcess)
	if err != nil {
		return interactionCheckpointState{}, fmt.Errorf("agentexec: Interaction checkpoint model contexts: %w", err)
	}
	pendingSteers, err := decodeInteractionPendingSteers(wire.PendingSteers)
	if err != nil {
		return interactionCheckpointState{}, fmt.Errorf("agentexec: Interaction checkpoint pending steers: %w", err)
	}
	pendingContinuation, err := decodeInteractionPendingContinuation(
		wire.PendingContinuation, tree.RootID(),
	)
	if err != nil {
		return interactionCheckpointState{}, fmt.Errorf("agentexec: Interaction checkpoint pending continuation: %w", err)
	}
	return interactionCheckpointState{
		tree: tree, callsByProcess: callsByProcess, carriedCallCount: carriedCallCount,
		contextByProcess: contextByProcess, instructions: instructions, pendingSteers: pendingSteers,
		pendingContinuation: pendingContinuation,
	}, nil
}

func decodeInteractionCheckpointWire(payload []byte) (interactionCheckpointPayloadWire, error) {
	if err := strictjson.ValidateUniqueMembers(payload); err != nil {
		return interactionCheckpointPayloadWire{}, fmt.Errorf("agentexec: decode Interaction checkpoint: %w", err)
	}
	var wire interactionCheckpointPayloadWire
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return interactionCheckpointPayloadWire{}, fmt.Errorf("agentexec: decode Interaction checkpoint: %w", err)
	}
	if wire.SchemaVersion != interactionCheckpointSchemaVersion {
		return interactionCheckpointPayloadWire{}, fmt.Errorf(
			"agentexec: Interaction checkpoint schema %d is not supported", wire.SchemaVersion,
		)
	}
	return wire, nil
}

func decodeInteractionCheckpointTree(
	wire json.RawMessage,
) (agent.TreeSnapshot, map[agent.ProcessID]struct{}, error) {
	tree, err := agent.ParseTreeSnapshot(wire)
	if err != nil {
		return agent.TreeSnapshot{}, nil, fmt.Errorf("agentexec: decode Interaction checkpoint tree: %w", err)
	}
	processes := make(map[agent.ProcessID]struct{}, len(tree.ProcessSnapshots()))
	for _, snapshot := range tree.ProcessSnapshots() {
		processes[snapshot.ProcessID()] = struct{}{}
	}
	return tree, processes, nil
}

func decodeInteractionCheckpointInstructions(messages []corechat.Message) ([]corechat.Message, error) {
	instructions := cloneChatMessages(messages)
	canonical, err := interactionInstructionContext(instructions)
	if err != nil || len(canonical) != len(instructions) {
		if err == nil {
			err = errors.New("instruction context contains a non-system message")
		}
		return nil, fmt.Errorf("agentexec: Interaction checkpoint instructions: %w", err)
	}
	return instructions, nil
}

func decodeInteractionCheckpointMembers(
	values []interactionMemberCallsWire,
	processes map[agent.ProcessID]struct{},
) (map[agent.ProcessID]map[string]int, error) {
	members := make(map[agent.ProcessID]map[string]int, len(values))
	previousMember := ""
	for index, member := range values {
		if index > 0 && member.MemberID <= previousMember {
			return nil, errors.New("agentexec: Interaction checkpoint members are not canonical")
		}
		processID, err := agent.ParseProcessID(member.MemberID)
		if err != nil {
			return nil, fmt.Errorf("agentexec: Interaction checkpoint member: %w", err)
		}
		if _, found := processes[processID]; !found {
			return nil, errors.New("agentexec: Interaction checkpoint accounting names a foreign member")
		}
		models, err := decodeInteractionCallCounts(member.Models)
		if err != nil || len(models) == 0 {
			if err == nil {
				err = errors.New("member call counts are empty")
			}
			return nil, fmt.Errorf("agentexec: Interaction checkpoint member %s: %w", processID, err)
		}
		members[processID] = models
		previousMember = member.MemberID
	}
	return members, nil
}

func decodeInteractionModelContexts(
	values []interactionModelContextWire,
	processes map[agent.ProcessID]struct{},
	callsByProcess map[agent.ProcessID]map[string]int,
) (map[agent.ProcessID]ModelContextTokenCalibration, error) {
	contexts := make(map[agent.ProcessID]ModelContextTokenCalibration, len(values))
	previous := ""
	for index, value := range values {
		if index > 0 && value.MemberID <= previous {
			return nil, errors.New("model contexts are not canonical")
		}
		processID, err := agent.ParseProcessID(value.MemberID)
		if err != nil {
			return nil, fmt.Errorf("model context member identity: %w", err)
		}
		if _, found := processes[processID]; !found {
			return nil, errors.New("model context names a foreign member")
		}
		if len(callsByProcess[processID]) == 0 {
			return nil, errors.New("model context has no matching member accounting")
		}
		calibration, err := NewModelContextTokenCalibration(
			value.ReportedTokens,
			value.EstimatedTokens,
		)
		if err != nil {
			return nil, err
		}
		contexts[processID] = calibration
		previous = value.MemberID
	}
	return contexts, nil
}

func decodeInteractionPendingSteers(
	values []interactionPendingSteerWire,
) (map[agent.SignalID]pendingInteractionSteer, error) {
	pending := make(map[agent.SignalID]pendingInteractionSteer, len(values))
	previous := ""
	for _, value := range values {
		signalID, steer, err := value.decode(previous)
		if err != nil {
			return nil, err
		}
		pending[signalID] = steer
		previous = value.SignalID
	}
	return pending, nil
}

func (w interactionPendingSteerWire) decode(
	previousSignalID string,
) (agent.SignalID, pendingInteractionSteer, error) {
	var zeroSignalID agent.SignalID
	if previousSignalID != "" && w.SignalID <= previousSignalID || len(w.Content) == 0 {
		return zeroSignalID, pendingInteractionSteer{}, errors.New("pending steers are not canonical")
	}
	signalID, err := agent.ParseSignalID(w.SignalID)
	if err != nil {
		return zeroSignalID, pendingInteractionSteer{}, fmt.Errorf("pending steer identity: %w", err)
	}
	content := make([]transcript.ContentBlock, len(w.Content))
	for index, block := range w.Content {
		content[index], err = block.decode()
		if err != nil {
			return zeroSignalID, pendingInteractionSteer{}, fmt.Errorf(
				"pending steer %s content %d: %w", signalID, index, err,
			)
		}
	}
	return signalID, pendingInteractionSteer{content: content}, nil
}

func decodeInteractionPendingContinuation(
	wire *interactionPendingContinuationWire,
	rootID agent.ProcessID,
) (*pendingInteractionContinuation, error) {
	if wire == nil {
		return nil, nil
	}
	processID, err := agent.ParseProcessID(wire.MemberID)
	if err != nil || processID != rootID {
		return nil, errors.New("pending continuation does not name the root member")
	}
	if _, err := resourceid.ParseItem(wire.ItemID); err != nil {
		return nil, fmt.Errorf("pending continuation Item: %w", err)
	}
	if len(wire.Content) == 0 {
		return nil, errors.New("pending continuation has no product content")
	}
	content := make([]transcript.ContentBlock, len(wire.Content))
	for index, block := range wire.Content {
		content[index], err = block.decode()
		if err != nil {
			return nil, fmt.Errorf("pending continuation content %d: %w", index, err)
		}
	}
	if _, err := runs.MaterializeUserMessage(content); err != nil {
		return nil, fmt.Errorf("pending continuation message: %w", err)
	}
	return &pendingInteractionContinuation{
		processID: processID, itemID: wire.ItemID, content: content,
	}, nil
}

func (w interactionContentBlockWire) decode() (transcript.ContentBlock, error) {
	switch w.Kind {
	case transcript.TextContent:
		return w.decodeText()
	case transcript.ImageContent:
		return w.decodeImage()
	default:
		return transcript.ContentBlock{}, fmt.Errorf("has unknown kind %q", w.Kind)
	}
}

func (w interactionContentBlockWire) decodeText() (transcript.ContentBlock, error) {
	if w.Text == "" || w.MediaType != "" || w.Data != "" {
		return transcript.ContentBlock{}, errors.New("is not canonical text")
	}
	content := transcript.ContentBlock{Kind: transcript.TextContent, Text: w.Text}
	if err := content.Validate(); err != nil {
		return transcript.ContentBlock{}, err
	}
	return content, nil
}

func (w interactionContentBlockWire) decodeImage() (transcript.ContentBlock, error) {
	if w.Text != "" || w.MediaType == "" || w.Data == "" {
		return transcript.ContentBlock{}, errors.New("is not canonical image")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(w.Data)
	if err != nil {
		return transcript.ContentBlock{}, fmt.Errorf("image data: %w", err)
	}
	if base64.StdEncoding.EncodeToString(data) != w.Data {
		return transcript.ContentBlock{}, errors.New("image data is not canonical base64")
	}
	content := transcript.ContentBlock{
		Kind: transcript.ImageContent, MediaType: w.MediaType, Bytes: data,
	}
	if err := content.Validate(); err != nil {
		return transcript.ContentBlock{}, err
	}
	return content, nil
}

func decodeInteractionCallCounts(values []interactionModelCallsWire) (map[string]int, error) {
	result := make(map[string]int, len(values))
	previous := ""
	for index, value := range values {
		if _, err := modelref.NewModelIdentity(value.Model); err != nil {
			return nil, fmt.Errorf("model call counts: models[%d]: %w", index, err)
		}
		if value.Calls <= 0 || index > 0 && value.Model <= previous {
			return nil, errors.New("model call counts are not canonical")
		}
		result[value.Model] = value.Calls
		previous = value.Model
	}
	return result, nil
}
