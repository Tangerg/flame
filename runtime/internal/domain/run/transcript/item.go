package transcript

import (
	"errors"
	"fmt"
	"time"

	flameidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

type ItemIdentity struct {
	SessionID  flameidentity.SessionID
	RunID      flameidentity.RunID
	ItemID     flameidentity.ItemID
	OccurredAt time.Time
}

func (i ItemIdentity) Validate() error {
	if err := i.SessionID.Validate(); err != nil {
		return fmt.Errorf("transcript: %w", err)
	}
	if err := i.RunID.Validate(); err != nil {
		return fmt.Errorf("transcript: %w", err)
	}
	if err := i.ItemID.Validate(); err != nil {
		return fmt.Errorf("transcript: %w", err)
	}
	if i.OccurredAt.IsZero() {
		return errors.New("transcript: occurrence time is required")
	}
	return instant.Validate(i.OccurredAt)
}

type Item struct {
	identity ItemIdentity
	status   ItemStatus
	body     Body
}

type ItemSnapshot struct {
	Identity ItemIdentity
	Status   ItemStatus
	Body     Body
}

func NewUserMessage(identity ItemIdentity, content []ContentBlock) (Item, error) {
	return RestoreItem(ItemSnapshot{Identity: identity, Status: ItemCompleted, Body: UserMessageBody{Content: content}})
}

func NewAgentMessage(identity ItemIdentity, phase MessagePhase, content []ContentBlock) (Item, error) {
	return RestoreItem(ItemSnapshot{
		Identity: identity, Status: ItemCompleted,
		Body: AgentMessageBody{Phase: phase, Content: content},
	})
}

func NewReasoning(identity ItemIdentity, text string, redacted bool) (Item, error) {
	return RestoreItem(ItemSnapshot{
		Identity: identity, Status: ItemCompleted,
		Body: ReasoningBody{Text: text, Redacted: redacted},
	})
}

func NewIncompleteAgentMessage(identity ItemIdentity, text string) (Item, error) {
	return RestoreItem(ItemSnapshot{
		Identity: identity, Status: ItemIncomplete,
		Body: AgentMessageBody{Phase: MessageCommentary, Content: []ContentBlock{{Kind: TextContent, Text: text}}},
	})
}

func NewIncompleteReasoning(identity ItemIdentity, text string) (Item, error) {
	return RestoreItem(ItemSnapshot{Identity: identity, Status: ItemIncomplete, Body: ReasoningBody{Text: text}})
}

func NewQuestion(identity ItemIdentity, question Question) (Item, error) {
	return RestoreItem(ItemSnapshot{Identity: identity, Status: ItemCompleted, Body: QuestionBody{Question: question}})
}

func NewToolCall(identity ItemIdentity, invocation ToolInvocation, safetyClass tool.SafetyClass) (Item, error) {
	return RestoreItem(ItemSnapshot{
		Identity: identity, Status: ItemRunning,
		Body: ToolCallBody{Invocation: invocation, SafetyClass: safetyClass},
	})
}

func NewCompaction(identity ItemIdentity, summary string, droppedMessages int) (Item, error) {
	return RestoreItem(ItemSnapshot{
		Identity: identity, Status: ItemCompleted,
		Body: CompactionBody{Summary: summary, DroppedMessages: droppedMessages},
	})
}

func RestoreItem(snapshot ItemSnapshot) (Item, error) {
	switch snapshot.Body.(type) {
	case nil:
		return Item{}, errors.New("transcript: Item body is required")
	case UserMessageBody, AgentMessageBody, ReasoningBody, QuestionBody, ToolCallBody, CompactionBody:
	default:
		return Item{}, errors.New("transcript: Item body must be a value variant")
	}
	snapshot.Identity.OccurredAt = snapshot.Identity.OccurredAt.UTC()
	item := Item{identity: snapshot.Identity, status: snapshot.Status, body: snapshot.Body.clone()}
	if err := item.validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (i Item) Snapshot() ItemSnapshot {
	return ItemSnapshot{Identity: i.identity, Status: i.status, Body: i.Body()}
}

func (i Item) Fork(sessionID flameidentity.SessionID, runID flameidentity.RunID, itemID flameidentity.ItemID) (Item, error) {
	if i.status == ItemRunning {
		return Item{}, errors.New("transcript: only a terminal Item can be forked")
	}
	snapshot := i.Snapshot()
	snapshot.Identity.SessionID = sessionID
	snapshot.Identity.RunID = runID
	snapshot.Identity.ItemID = itemID
	return RestoreItem(snapshot)
}

func (i Item) CompleteToolCall(
	invocation ToolInvocation,
	offload *ToolResultOffload,
	executionStartedAt time.Time,
	finishedAt time.Time,
) (Replacement, error) {
	return i.settleToolCall(invocation, offload, nil, ItemCompleted, executionStartedAt, finishedAt)
}

func (i Item) FailToolCall(
	invocation ToolInvocation,
	offload *ToolResultOffload,
	failure tool.Failure,
	executionStartedAt time.Time,
	finishedAt time.Time,
) (Replacement, error) {
	return i.settleToolCall(invocation, offload, &failure, ItemIncomplete, executionStartedAt, finishedAt)
}

func (i Item) AbandonToolCall(failure *tool.Failure, finishedAt time.Time) (Replacement, error) {
	call, ok := i.body.(ToolCallBody)
	if !ok {
		return Replacement{}, errors.New("transcript: only a ToolCall can be abandoned")
	}
	return i.settleToolCall(call.Invocation, nil, failure, ItemIncomplete, time.Time{}, finishedAt)
}

func (i Item) AbandonStartedToolCall(
	invocation ToolInvocation,
	failure *tool.Failure,
	executionStartedAt time.Time,
	finishedAt time.Time,
) (Replacement, error) {
	return i.settleToolCall(invocation, nil, failure, ItemIncomplete, executionStartedAt, finishedAt)
}

func (i Item) ResolveToolApproval(resolution approval.Resolution) (Replacement, error) {
	call, ok := i.body.(ToolCallBody)
	if !ok || i.status != ItemRunning {
		return Replacement{}, errors.New("transcript: only a running ToolCall can resolve approval")
	}
	if call.Approval != nil {
		return Replacement{}, errors.New("transcript: ToolCall approval is already resolved")
	}
	call.Approval = &resolution
	return i.replacedBy(i.with(i.status, call))
}

func (i Item) AnswerQuestion(answers [][]string) (Replacement, error) {
	prompt, ok := i.body.(QuestionBody)
	if !ok {
		return Replacement{}, errors.New("transcript: only a Question can be answered")
	}
	if prompt.Question.Answered() {
		return Replacement{}, errors.New("transcript: Question is already answered")
	}
	if answers == nil {
		return Replacement{}, errors.New("transcript: accepted Question answers are required")
	}
	prompt.Question.Answers = answers
	return i.replacedBy(i.with(i.status, prompt))
}

func (i Item) settleToolCall(
	invocation ToolInvocation,
	offload *ToolResultOffload,
	failure *tool.Failure,
	status ItemStatus,
	executionStartedAt time.Time,
	finishedAt time.Time,
) (Replacement, error) {
	call, ok := i.body.(ToolCallBody)
	if !ok || i.status != ItemRunning {
		return Replacement{}, errors.New("transcript: only a running ToolCall can settle")
	}
	if invocation.Source == (tool.Ref{}) {
		invocation.Source = call.Invocation.Source
	}
	if !call.Invocation.MatchesSource(invocation.Name, invocation.Source) {
		return Replacement{}, errors.New("transcript: ToolCall settlement changes tool identity")
	}
	if finishedAt.IsZero() || finishedAt.Before(i.identity.OccurredAt) {
		return Replacement{}, errors.New("transcript: ToolCall finish time precedes its occurrence")
	}
	var executionDuration *time.Duration
	if !executionStartedAt.IsZero() {
		if executionStartedAt.Before(i.identity.OccurredAt) || finishedAt.Before(executionStartedAt) {
			return Replacement{}, errors.New("transcript: ToolCall execution interval is outside its lifecycle")
		}
		executionDuration = new(finishedAt.Sub(executionStartedAt))
	} else if status == ItemCompleted {
		return Replacement{}, errors.New("transcript: completed ToolCall requires an execution interval")
	}
	if invocation.Offload != nil {
		return Replacement{}, errors.New("transcript: ToolCall settlement derives its reference from the offload body")
	}
	var blob *ToolResultBlob
	if offload != nil {
		invocation.Offload = &offload.Ref
		body := ToolResultBlob{ID: offload.Ref.ID, SessionID: i.SessionID(), Body: offload.Body, CreatedAt: finishedAt.UTC()}
		if err := body.Validate(); err != nil {
			return Replacement{}, err
		}
		blob = &body
	}
	call.Invocation = invocation
	call.FinishedAt = finishedAt.UTC()
	call.ExecutionDuration = executionDuration
	call.Failure = failure
	state, err := i.with(status, call)
	if err != nil {
		return Replacement{}, err
	}
	return Replacement{expected: i, state: state, toolResult: blob}, nil
}

func (i Item) with(status ItemStatus, body Body) (Item, error) {
	return RestoreItem(ItemSnapshot{Identity: i.identity, Status: status, Body: body})
}

func (i Item) validate() error {
	if err := i.identity.Validate(); err != nil {
		return err
	}
	if !i.status.Valid() {
		return fmt.Errorf("transcript: unknown Item status %q", i.status)
	}
	return i.body.validate(i.status, i.identity.OccurredAt)
}

// ValidateSpawns enforces where a child Run's spawning call may live: a tool
// call of the parent Run in the same Session.
func (i Item) ValidateSpawns(sessionID flameidentity.SessionID, parentRunID flameidentity.RunID) error {
	switch {
	case i.Kind() != ToolCall:
		return fmt.Errorf("transcript: spawning Item %q is not a tool call", i.ID())
	case i.SessionID() != sessionID:
		return fmt.Errorf("transcript: spawning Item %q belongs to Session %q, want %q", i.ID(), i.SessionID(), sessionID)
	case i.RunID() != parentRunID:
		return fmt.Errorf("transcript: spawning Item %q belongs to Run %q, want parent Run %q", i.ID(), i.RunID(), parentRunID)
	}
	return nil
}

func (i Item) Identity() ItemIdentity             { return i.identity }
func (i Item) SessionID() flameidentity.SessionID { return i.identity.SessionID }
func (i Item) RunID() flameidentity.RunID         { return i.identity.RunID }
func (i Item) ID() flameidentity.ItemID           { return i.identity.ItemID }
func (i Item) Status() ItemStatus                 { return i.status }
func (i Item) OccurredAt() time.Time              { return i.identity.OccurredAt }
func (i Item) Kind() ItemKind                     { return i.body.Kind() }

func (i Item) Body() Body { return i.body.clone() }

func (i Item) ToolCall() (ToolCallBody, bool) {
	call, ok := i.body.(ToolCallBody)
	if !ok {
		return ToolCallBody{}, false
	}
	return call.cloned(), true
}

func (i Item) Question() (Question, bool) {
	prompt, ok := i.body.(QuestionBody)
	if !ok {
		return Question{}, false
	}
	return cloneQuestion(prompt.Question), true
}

func (i Item) Equal(other Item) bool {
	return i.identity.SessionID == other.identity.SessionID &&
		i.identity.RunID == other.identity.RunID &&
		i.identity.ItemID == other.identity.ItemID &&
		i.identity.OccurredAt.Equal(other.identity.OccurredAt) &&
		i.status == other.status &&
		i.body.equal(other.body)
}
