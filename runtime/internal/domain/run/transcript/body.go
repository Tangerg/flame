package transcript

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// Body is the closed set of Item variants. The variant alone decides the
// Item's Kind, so no field of one variant can appear on another.
type Body interface {
	Kind() ItemKind
	validate(status ItemStatus, occurredAt time.Time) error
	clone() Body
	equal(Body) bool
}

type UserMessageBody struct {
	Content []ContentBlock
}

type AgentMessageBody struct {
	Phase   MessagePhase
	Content []ContentBlock
}

type ReasoningBody struct {
	Text     string
	Redacted bool
}

type QuestionBody struct {
	Question Question
}

// ToolCallBody is the only variant with a lifecycle: the terminal facts are
// absent while running and recorded once by settlement.
type ToolCallBody struct {
	Invocation  ToolInvocation
	SafetyClass tool.SafetyClass
	// Approval is the accepted human verdict; nil means none was required or
	// it is still unresolved.
	Approval          *approval.Resolution
	FinishedAt        time.Time
	ExecutionDuration *time.Duration
	Failure           *tool.Failure
}

type CompactionBody struct {
	Summary         string
	DroppedMessages int
}

func (UserMessageBody) Kind() ItemKind  { return UserMessage }
func (AgentMessageBody) Kind() ItemKind { return AgentMessage }
func (ReasoningBody) Kind() ItemKind    { return Reasoning }
func (QuestionBody) Kind() ItemKind     { return QuestionItem }
func (ToolCallBody) Kind() ItemKind     { return ToolCall }
func (CompactionBody) Kind() ItemKind   { return Compaction }

func (p UserMessageBody) validate(status ItemStatus, _ time.Time) error {
	if status != ItemCompleted {
		return errors.New("transcript: userMessage must be complete")
	}
	return validateMessageContent(p.Content)
}

// Visible model observations can settle incomplete without becoming a response.
func (p AgentMessageBody) validate(status ItemStatus, _ time.Time) error {
	if status != ItemCompleted && status != ItemIncomplete {
		return errors.New("transcript: agentMessage must be complete or incomplete")
	}
	if !p.Phase.Valid() {
		return fmt.Errorf("transcript: unknown AgentMessage phase %q", p.Phase)
	}
	return validateMessageContent(p.Content)
}

func (p ReasoningBody) validate(status ItemStatus, _ time.Time) error {
	if status != ItemCompleted && status != ItemIncomplete {
		return errors.New("transcript: reasoning must be complete or incomplete")
	}
	if p.Text == "" {
		return errors.New("transcript: reasoning has no text")
	}
	return nil
}

func (p QuestionBody) validate(status ItemStatus, _ time.Time) error {
	if status != ItemCompleted {
		return errors.New("transcript: question must be complete")
	}
	return p.Question.Validate()
}

func (p ToolCallBody) validate(status ItemStatus, occurredAt time.Time) error {
	if err := instant.Validate(p.FinishedAt); err != nil {
		return fmt.Errorf("transcript: %w", err)
	}
	if err := p.Invocation.Validate(status == ItemRunning); err != nil {
		return err
	}
	if p.SafetyClass != "" && !p.SafetyClass.Valid() {
		return fmt.Errorf("transcript: unknown Tool safety class %q", p.SafetyClass)
	}
	if p.Approval != nil {
		if err := p.Approval.Validate(); err != nil {
			return err
		}
	}
	switch status {
	case ItemRunning:
		if !p.FinishedAt.IsZero() || p.ExecutionDuration != nil || p.Failure != nil {
			return errors.New("transcript: running ToolCall carries terminal facts")
		}
	case ItemCompleted:
		if p.FinishedAt.IsZero() || p.ExecutionDuration == nil || p.Failure != nil {
			return errors.New("transcript: completed ToolCall has invalid terminal facts")
		}
	case ItemIncomplete:
		if p.FinishedAt.IsZero() {
			return errors.New("transcript: incomplete ToolCall has no finish time")
		}
	}
	if !p.FinishedAt.IsZero() && p.FinishedAt.Before(occurredAt) {
		return errors.New("transcript: ToolCall finish time precedes its occurrence")
	}
	if p.ExecutionDuration != nil {
		if *p.ExecutionDuration < 0 {
			return errors.New("transcript: ToolCall execution duration is negative")
		}
		if *p.ExecutionDuration > p.FinishedAt.Sub(occurredAt) {
			return errors.New("transcript: ToolCall execution duration exceeds its lifecycle")
		}
	}
	if p.Failure != nil {
		if err := p.Failure.Validate(); err != nil {
			return fmt.Errorf("transcript: Tool failure: %w", err)
		}
	}
	return nil
}

func (p CompactionBody) validate(status ItemStatus, _ time.Time) error {
	if status != ItemCompleted {
		return errors.New("transcript: compaction must be complete")
	}
	if strings.TrimSpace(p.Summary) == "" {
		return errors.New("transcript: compaction summary is required")
	}
	if p.Summary != strings.TrimSpace(p.Summary) {
		return errors.New("transcript: compaction summary is not canonical")
	}
	if p.DroppedMessages < 0 {
		return errors.New("transcript: dropped messages must not be negative")
	}
	return nil
}

func validateMessageContent(content []ContentBlock) error {
	if len(content) == 0 {
		return errors.New("transcript: message has no content")
	}
	for index, block := range content {
		if err := block.Validate(); err != nil {
			return fmt.Errorf("transcript: content %d: %w", index, err)
		}
	}
	return nil
}

func (p UserMessageBody) clone() Body {
	return UserMessageBody{Content: CloneContent(p.Content)}
}

func (p AgentMessageBody) clone() Body {
	return AgentMessageBody{Phase: p.Phase, Content: CloneContent(p.Content)}
}

func (p ReasoningBody) clone() Body { return p }

func (p QuestionBody) clone() Body {
	return QuestionBody{Question: cloneQuestion(p.Question)}
}

func (p ToolCallBody) clone() Body {
	p.Invocation = p.Invocation.clone()
	if p.Approval != nil {
		p.Approval = new(*p.Approval)
	}
	if p.ExecutionDuration != nil {
		p.ExecutionDuration = new(*p.ExecutionDuration)
	}
	if p.Failure != nil {
		p.Failure = new(*p.Failure)
	}
	return p
}

func (p CompactionBody) clone() Body { return p }

func (p UserMessageBody) equal(other Body) bool {
	o, ok := other.(UserMessageBody)
	return ok && slices.EqualFunc(p.Content, o.Content, ContentBlock.Equal)
}

func (p AgentMessageBody) equal(other Body) bool {
	o, ok := other.(AgentMessageBody)
	return ok && p.Phase == o.Phase && slices.EqualFunc(p.Content, o.Content, ContentBlock.Equal)
}

func (p ReasoningBody) equal(other Body) bool {
	o, ok := other.(ReasoningBody)
	return ok && p == o
}

func (p QuestionBody) equal(other Body) bool {
	o, ok := other.(QuestionBody)
	return ok && p.Question.Equal(o.Question)
}

func (p ToolCallBody) equal(other Body) bool {
	o, ok := other.(ToolCallBody)
	return ok && p.Invocation.Equal(o.Invocation) && p.SafetyClass == o.SafetyClass &&
		equalPointers(p.Approval, o.Approval) && p.FinishedAt.Equal(o.FinishedAt) &&
		equalPointers(p.ExecutionDuration, o.ExecutionDuration) && equalPointers(p.Failure, o.Failure)
}

func (p CompactionBody) equal(other Body) bool {
	o, ok := other.(CompactionBody)
	return ok && p == o
}

func equalPointers[T comparable](left, right *T) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

// Validate reports whether an invocation has a canonical identity and result
// shape. pending requires the result and offload reference to be absent.
func (t ToolInvocation) Validate(pending bool) error {
	if t.ArgumentsText != "" && !t.Arguments.Equal(tool.Arguments{}) {
		return errors.New("transcript: Tool invocation carries both parsed and rejected arguments")
	}
	if strings.TrimSpace(t.Name) == "" || t.Name != strings.TrimSpace(t.Name) {
		return errors.New("transcript: Tool name is required without surrounding whitespace")
	}
	if pending && (t.Result != nil || t.Offload != nil) {
		return errors.New("transcript: pending Tool invocation carries a result")
	}
	if t.Offload != nil {
		if err := t.Offload.Validate(); err != nil {
			return fmt.Errorf("transcript: Tool offload: %w", err)
		}
		if t.Result == nil {
			return errors.New("transcript: offloaded Tool result has no preview")
		}
		if _, textual := t.Result.String(); !textual {
			return errors.New("transcript: offloaded Tool result preview is not text")
		}
	}
	return nil
}

func (t ToolInvocation) clone() ToolInvocation {
	if t.Result != nil {
		t.Result = new(*t.Result)
	}
	if t.Offload != nil {
		t.Offload = new(*t.Offload)
	}
	return t
}

func cloneQuestion(question Question) Question {
	cloned := Question{
		Fields:  make([]QuestionField, len(question.Fields)),
		Answers: CloneAnswers(question.Answers),
	}
	for index, field := range question.Fields {
		cloned.Fields[index] = field
		cloned.Fields[index].Options = append([]QuestionOption(nil), field.Options...)
	}
	return cloned
}

// CloneAnswers copies one question's answers away from their producer.
//
// A row is appended onto a nil slice rather than cloned, so an answered-but-empty
// field arrives as nil and encodes as JSON null. That is the published shape for
// a skipped field: slices.Clone would preserve the empty slice and publish [],
// changing the wire for every consumer that reads a skipped answer back.
func CloneAnswers(answers [][]string) [][]string {
	if answers == nil {
		return nil
	}
	cloned := make([][]string, len(answers))
	for index, values := range answers {
		cloned[index] = append([]string(nil), values...)
	}
	return cloned
}
