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

// ItemIdentity is the immutable ownership and occurrence identity shared by
// every transcript Item variant.
type ItemIdentity struct {
	SessionID  string
	RunID      string
	ItemID     string
	OccurredAt time.Time
}

// Validate reports whether the identity is complete and canonical.
func (i ItemIdentity) Validate() error {
	if err := flameidentity.ValidateSession(i.SessionID); err != nil {
		return fmt.Errorf("transcript: %w", err)
	}
	if err := flameidentity.ValidateRun(i.RunID); err != nil {
		return fmt.Errorf("transcript: %w", err)
	}
	if err := flameidentity.ValidateItem(i.ItemID); err != nil {
		return fmt.Errorf("transcript: %w", err)
	}
	if i.OccurredAt.IsZero() {
		return errors.New("transcript: occurrence time is required")
	}
	return instant.Validate(i.OccurredAt)
}

// Item is one immutable, user-visible transcript fact. Only ToolCall has an
// internal status lifecycle. A Question prompt is complete when constructed and
// may later be replaced by the same fact enriched with its accepted answer.
type Item struct {
	identity ItemIdentity
	status   ItemStatus
	body     Body
}

// ItemSnapshot is the complete representation consumed by strict technical
// codecs. [RestoreItem] validates it; it is not a second mutation surface.
type ItemSnapshot struct {
	Identity ItemIdentity
	Status   ItemStatus
	Body     Body
}

// NewUserMessage constructs a complete user message.
func NewUserMessage(identity ItemIdentity, content []ContentBlock) (Item, error) {
	return RestoreItem(ItemSnapshot{Identity: identity, Status: ItemCompleted, Body: UserMessageBody{Content: content}})
}

// NewAgentMessage constructs a complete agent message.
func NewAgentMessage(identity ItemIdentity, phase MessagePhase, content []ContentBlock) (Item, error) {
	return RestoreItem(ItemSnapshot{
		Identity: identity, Status: ItemCompleted,
		Body: AgentMessageBody{Phase: phase, Content: content},
	})
}

// NewReasoning constructs a complete reasoning record.
func NewReasoning(identity ItemIdentity, text string, redacted bool) (Item, error) {
	return RestoreItem(ItemSnapshot{
		Identity: identity, Status: ItemCompleted,
		Body: ReasoningBody{Text: text, Redacted: redacted},
	})
}

// NewIncompleteAgentMessage retains visible output from an unfinished response.
func NewIncompleteAgentMessage(identity ItemIdentity, text string) (Item, error) {
	return RestoreItem(ItemSnapshot{
		Identity: identity, Status: ItemIncomplete,
		Body: AgentMessageBody{Phase: MessageCommentary, Content: []ContentBlock{{Kind: TextContent, Text: text}}},
	})
}

// NewIncompleteReasoning retains visible reasoning without provider replay state.
func NewIncompleteReasoning(identity ItemIdentity, text string) (Item, error) {
	return RestoreItem(ItemSnapshot{Identity: identity, Status: ItemIncomplete, Body: ReasoningBody{Text: text}})
}

// NewQuestion constructs the complete prompt fact for one pending question.
// Whether it still awaits an answer belongs to the root-owned Pending set, not
// to a second lifecycle on the transcript Item.
func NewQuestion(identity ItemIdentity, question Question) (Item, error) {
	return RestoreItem(ItemSnapshot{Identity: identity, Status: ItemCompleted, Body: QuestionBody{Question: question}})
}

// NewToolCall constructs a running ToolCall. Its invocation must not already
// carry a result or offload reference.
func NewToolCall(identity ItemIdentity, invocation ToolInvocation, safetyClass tool.SafetyClass) (Item, error) {
	return RestoreItem(ItemSnapshot{
		Identity: identity, Status: ItemRunning,
		Body: ToolCallBody{Invocation: invocation, SafetyClass: safetyClass},
	})
}

// NewCompaction constructs a complete compaction boundary.
func NewCompaction(identity ItemIdentity, summary string, droppedMessages int) (Item, error) {
	return RestoreItem(ItemSnapshot{
		Identity: identity, Status: ItemCompleted,
		Body: CompactionBody{Summary: summary, DroppedMessages: droppedMessages},
	})
}

// RestoreItem rebuilds a complete Item from a technical record.
func RestoreItem(snapshot ItemSnapshot) (Item, error) {
	if snapshot.Body == nil {
		return Item{}, errors.New("transcript: Item body is required")
	}
	snapshot.Identity.OccurredAt = snapshot.Identity.OccurredAt.UTC()
	item := Item{identity: snapshot.Identity, status: snapshot.Status, body: snapshot.Body.clone()}
	if err := item.validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

// Snapshot returns a complete ownership-isolated representation.
func (i Item) Snapshot() ItemSnapshot {
	return ItemSnapshot{Identity: i.identity, Status: i.status, Body: i.Body()}
}

// Fork derives a terminal historical Item for a child Session under fresh
// ownership identities. Semantic content and lifecycle times are preserved. An
// offloaded Tool result keeps its id: ids are Session-scoped, and the preview
// spells the id out, so the child Session holds the body under the same one.
func (i Item) Fork(sessionID flameidentity.SessionID, runID flameidentity.RunID, itemID string) (Item, error) {
	if i.status == ItemRunning {
		return Item{}, errors.New("transcript: only a terminal Item can be forked")
	}
	snapshot := i.Snapshot()
	snapshot.Identity.SessionID = sessionID.String()
	snapshot.Identity.RunID = runID.String()
	snapshot.Identity.ItemID = itemID
	return RestoreItem(snapshot)
}

// CompleteToolCall settles a running ToolCall successfully with the exact
// execution interval reported by the Tool boundary.
func (i Item) CompleteToolCall(
	invocation ToolInvocation,
	executionStartedAt time.Time,
	finishedAt time.Time,
) (Replacement, error) {
	return i.replacedBy(i.settleToolCall(invocation, nil, ItemCompleted, executionStartedAt, finishedAt))
}

// FailToolCall settles a started ToolCall with a classified failure and the
// exact execution interval reported by the Tool boundary.
func (i Item) FailToolCall(
	invocation ToolInvocation,
	failure tool.Failure,
	executionStartedAt time.Time,
	finishedAt time.Time,
) (Replacement, error) {
	return i.replacedBy(i.settleToolCall(invocation, &failure, ItemIncomplete, executionStartedAt, finishedAt))
}

// AbandonToolCall records that a running ToolCall never reached a conclusive
// result. Failure may be absent when only executor loss is known.
func (i Item) AbandonToolCall(failure *tool.Failure, finishedAt time.Time) (Replacement, error) {
	call, ok := i.body.(ToolCallBody)
	if !ok {
		return Replacement{}, errors.New("transcript: only a ToolCall can be abandoned")
	}
	return i.replacedBy(i.settleToolCall(call.Invocation, failure, ItemIncomplete, time.Time{}, finishedAt))
}

// AbandonStartedToolCall records an inconclusive started Tool attempt whose
// executor boundary still supplied its admitted input and exact interval.
func (i Item) AbandonStartedToolCall(
	invocation ToolInvocation,
	failure *tool.Failure,
	executionStartedAt time.Time,
	finishedAt time.Time,
) (Replacement, error) {
	return i.replacedBy(i.settleToolCall(invocation, failure, ItemIncomplete, executionStartedAt, finishedAt))
}

// ResolveToolApproval records the exact human verdict accepted for a running
// ToolCall. The decision is a durable semantic fact on the invocation rather
// than a property of the current policy or its eventual execution outcome.
// Identity and lifecycle remain unchanged, and a second verdict is rejected.
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

// AnswerQuestion enriches one complete Question prompt with the exact response
// accepted at the resume linearization point. Identity, occurrence, and Item
// status remain unchanged; a second answer is rejected.
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
	failure *tool.Failure,
	status ItemStatus,
	executionStartedAt time.Time,
	finishedAt time.Time,
) (Item, error) {
	call, ok := i.body.(ToolCallBody)
	if !ok || i.status != ItemRunning {
		return Item{}, errors.New("transcript: only a running ToolCall can settle")
	}
	if invocation.Name != call.Invocation.Name {
		return Item{}, errors.New("transcript: ToolCall settlement changes tool identity")
	}
	if finishedAt.IsZero() || finishedAt.Before(i.identity.OccurredAt) {
		return Item{}, errors.New("transcript: ToolCall finish time precedes its occurrence")
	}
	var executionDuration *time.Duration
	if !executionStartedAt.IsZero() {
		if executionStartedAt.Before(i.identity.OccurredAt) || finishedAt.Before(executionStartedAt) {
			return Item{}, errors.New("transcript: ToolCall execution interval is outside its lifecycle")
		}
		executionDuration = new(finishedAt.Sub(executionStartedAt))
	} else if status == ItemCompleted {
		return Item{}, errors.New("transcript: completed ToolCall requires an execution interval")
	}
	call.Invocation = invocation
	call.FinishedAt = finishedAt.UTC()
	call.ExecutionDuration = executionDuration
	call.Failure = failure
	return i.with(status, call)
}

// with is the one exit of every transition: the successor is validated like a
// restored Item, so a transition cannot produce what restore would reject.
func (i Item) with(status ItemStatus, body Body) (Item, error) {
	return RestoreItem(ItemSnapshot{Identity: i.identity, Status: status, Body: body})
}

// validate reports whether the Item is one legal variant. Every constructor
// and transition ends here, so an Item that exists is already legal and no
// reader has to ask again.
func (i Item) validate() error {
	if err := i.identity.Validate(); err != nil {
		return err
	}
	if !i.status.Valid() {
		return fmt.Errorf("transcript: unknown Item status %q", i.status)
	}
	return i.body.validate(i.status, i.identity.OccurredAt)
}

func (i Item) SessionID() string     { return i.identity.SessionID }
func (i Item) RunID() string         { return i.identity.RunID }
func (i Item) ID() string            { return i.identity.ItemID }
func (i Item) Status() ItemStatus    { return i.status }
func (i Item) OccurredAt() time.Time { return i.identity.OccurredAt }
func (i Item) Kind() ItemKind        { return i.body.Kind() }

// Body returns an ownership-isolated copy of the Item's variant.
func (i Item) Body() Body { return i.body.clone() }

// ToolCall returns the ToolCall variant when the Item is one.
func (i Item) ToolCall() (ToolCallBody, bool) {
	call, ok := i.body.(ToolCallBody)
	if !ok {
		return ToolCallBody{}, false
	}
	return call.clone().(ToolCallBody), true
}

// Question returns the Question variant's prompt when the Item is one.
func (i Item) Question() (Question, bool) {
	prompt, ok := i.body.(QuestionBody)
	if !ok {
		return Question{}, false
	}
	return cloneQuestion(prompt.Question), true
}

// Equal compares the complete semantic Item, including accepted answers and
// terminal execution evidence, independently of codec slice representation.
func (i Item) Equal(other Item) bool {
	return i.identity.SessionID == other.identity.SessionID &&
		i.identity.RunID == other.identity.RunID &&
		i.identity.ItemID == other.identity.ItemID &&
		i.identity.OccurredAt.Equal(other.identity.OccurredAt) &&
		i.status == other.status &&
		i.body.equal(other.body)
}
