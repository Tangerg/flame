package transcript

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

var ErrIdentityConflict = errors.New("transcript: identity conflict")

type ItemStatus string

const (
	ItemRunning    ItemStatus = "running"
	ItemCompleted  ItemStatus = "completed"
	ItemIncomplete ItemStatus = "incomplete"
)

func (i ItemStatus) Valid() bool {
	return i == ItemRunning || i == ItemCompleted || i == ItemIncomplete
}

type ItemKind string

const (
	UserMessage  ItemKind = "userMessage"
	AgentMessage ItemKind = "agentMessage"
	Reasoning    ItemKind = "reasoning"
	QuestionItem ItemKind = "question"
	ToolCall     ItemKind = "toolCall"
	Compaction   ItemKind = "compaction"
)

func (i ItemKind) Valid() bool {
	return i == UserMessage || i == AgentMessage || i == Reasoning ||
		i == QuestionItem || i == ToolCall || i == Compaction
}

type MessagePhase string

const (
	MessagePhaseNone   MessagePhase = ""
	MessageCommentary  MessagePhase = "commentary"
	MessageFinalAnswer MessagePhase = "finalAnswer"
)

func (m MessagePhase) Valid() bool {
	return m == MessageCommentary || m == MessageFinalAnswer
}

// The store assigns the position when the Item lands, so it sits beside the Item
// rather than as a zero every writer must ignore; occurrence timestamps tie and
// imports can backdate them.
type SequencedItem struct {
	Sequence int64
	Item     Item
}

type SequenceOrder string

const (
	OldestFirst SequenceOrder = "oldest"
	NewestFirst SequenceOrder = "newest"
)

func (s SequenceOrder) Valid() bool {
	return s == OldestFirst || s == NewestFirst
}

func (s SequenceOrder) Validate() error {
	if !s.Valid() {
		return fmt.Errorf("transcript: invalid sequence order %q", s)
	}
	return nil
}

func (s SequenceOrder) String() string { return string(s) }

type ContentKind string

const (
	TextContent  ContentKind = "text"
	ImageContent ContentKind = "image"
)

type ContentBlock struct {
	Kind      ContentKind
	Text      string
	MediaType string
	Bytes     []byte
}

func (c ContentBlock) Equal(other ContentBlock) bool {
	return c.Kind == other.Kind && c.Text == other.Text && c.MediaType == other.MediaType && bytes.Equal(c.Bytes, other.Bytes)
}

func (c ContentBlock) Clone() ContentBlock {
	c.Bytes = slices.Clone(c.Bytes)
	return c
}

func CloneContent(blocks []ContentBlock) []ContentBlock {
	cloned := make([]ContentBlock, len(blocks))
	for index, block := range blocks {
		cloned[index] = block.Clone()
	}
	return cloned
}

type Question struct {
	Fields  []QuestionField
	Answers [][]string
}

const MaximumQuestionHeaderCharacters = 12

type QuestionField struct {
	Prompt      string
	Header      string
	Kind        questioncontract.Kind
	Options     []QuestionOption
	Multiple    bool
	AllowCustom bool
}

type QuestionOption struct {
	Label       string
	Description string
	Preview     string
}

type ToolInvocation struct {
	Source  tool.Ref
	Name    string
	Input   ToolInput
	Result  *tool.Result
	Offload *ToolResultRef
}

type ToolInput struct {
	arguments tool.Arguments
	rejected  string
}

func AdmittedInput(arguments tool.Arguments) ToolInput { return ToolInput{arguments: arguments} }

func RejectedInput(text string) ToolInput { return ToolInput{rejected: text} }

func (i ToolInput) Arguments() tool.Arguments { return i.arguments }

func (i ToolInput) Rejected() (string, bool) { return i.rejected, i.rejected != "" }

func (i ToolInput) Equal(other ToolInput) bool {
	return i.rejected == other.rejected && i.arguments.Equal(other.arguments)
}

func (t ToolInvocation) MatchesSource(name string, source tool.Ref) bool {
	return t.Name == name && (t.Source == (tool.Ref{}) || source == (tool.Ref{}) || t.Source == source)
}

func (t ToolInvocation) Equal(other ToolInvocation) bool {
	if t.Name != other.Name || t.Source != other.Source || !t.Input.Equal(other.Input) {
		return false
	}
	if (t.Result == nil) != (other.Result == nil) {
		return false
	}
	if t.Result != nil && !t.Result.Equal(*other.Result) {
		return false
	}
	if (t.Offload == nil) != (other.Offload == nil) {
		return false
	}
	return t.Offload == nil || *t.Offload == *other.Offload
}

type Interrupt struct {
	ItemID         identity.ItemID
	ItemOccurredAt time.Time
	// The Run that raised this interrupt, not the set's owner: recorded rather than
	// derived because the derivation fails exactly when a tree has several Runs.
	RunID  identity.RunID
	Prompt Prompt
}

type Prompt interface {
	Type() run.InterruptType
	prompt()
}

func (Approval) Type() run.InterruptType { return run.InterruptApproval }
func (Question) Type() run.InterruptType { return run.InterruptQuestion }

func (Approval) prompt() {}
func (Question) prompt() {}

func (i Interrupt) Type() run.InterruptType { return i.Prompt.Type() }

func (i Interrupt) Validate() error {
	if err := i.ItemID.Validate(); err != nil {
		return fmt.Errorf("pending interrupt: %w", err)
	}
	if i.ItemOccurredAt.IsZero() {
		return errors.New("item occurrence time is required")
	}
	if err := i.RunID.Validate(); err != nil {
		return fmt.Errorf("pending interrupt: %w", err)
	}
	switch prompt := i.Prompt.(type) {
	case Approval:
		return prompt.Validate()
	case Question:
		if err := prompt.Validate(); err != nil {
			return err
		}
		if prompt.Answered() {
			return errors.New("open question interrupt already carries an accepted answer")
		}
		return nil
	default:
		return errors.New("interrupt has no prompt")
	}
}

type Approval struct {
	Tool         ToolInvocation
	Risk         tool.RiskLevel
	Reason       string
	Rememberable bool
}

func (a Approval) Validate() error {
	switch {
	case strings.TrimSpace(a.Tool.Name) == "":
		return errors.New("approval tool name is required")
	case a.Tool.Name != strings.TrimSpace(a.Tool.Name):
		return errors.New("approval tool name has surrounding whitespace")
	case a.Tool.Result != nil:
		return errors.New("approval tool must not carry a result")
	case a.Tool.Offload != nil:
		return errors.New("approval tool must not carry an offload reference")
	case !a.Risk.Valid():
		return fmt.Errorf("approval has unknown risk %q", a.Risk)
	}
	return nil
}

const imageMediaTypePrefix = "image/"

type ContentField uint8

const (
	ContentKindField ContentField = iota
	ContentTextField
	ContentMediaTypeField
	ContentBytesField
)

type ContentError struct {
	Field  ContentField
	Detail string
}

func (e *ContentError) Error() string { return "transcript: content " + e.Detail }

func (c ContentBlock) Validate() error {
	switch c.Kind {
	case TextContent:
		if strings.TrimSpace(c.Text) == "" {
			return &ContentError{Field: ContentTextField, Detail: "must contain non-whitespace text"}
		}
		if c.MediaType != "" || len(c.Bytes) != 0 {
			return &ContentError{Field: ContentKindField, Detail: "text content cannot carry media"}
		}
	case ImageContent:
		if c.Text != "" {
			return &ContentError{Field: ContentKindField, Detail: "image content cannot carry text"}
		}
		if !strings.HasPrefix(c.MediaType, imageMediaTypePrefix) {
			return &ContentError{Field: ContentMediaTypeField, Detail: fmt.Sprintf("must be an image media type, got %q", c.MediaType)}
		}
		if len(c.Bytes) == 0 {
			return &ContentError{Field: ContentBytesField, Detail: "must not be empty"}
		}
	default:
		return &ContentError{Field: ContentKindField, Detail: fmt.Sprintf("kind %q is unknown", c.Kind)}
	}
	return nil
}

func (q Question) Validate() error {
	contract, err := q.AnswerContract()
	if err != nil {
		return err
	}
	if q.Answers != nil {
		return contract.Validate(q.Answers)
	}
	return nil
}

func (q Question) AnswerContract() (questioncontract.Contract, error) {
	if err := questioncontract.ValidateFieldCount(len(q.Fields)); err != nil {
		return questioncontract.Contract{}, err
	}
	fields := make([]questioncontract.Field, len(q.Fields))
	for index, field := range q.Fields {
		if err := field.validate(index); err != nil {
			return questioncontract.Contract{}, err
		}
		labels := make([]string, len(field.Options))
		for i, option := range field.Options {
			labels[i] = option.Label
		}
		fields[index] = questioncontract.Field{Kind: field.Kind, Options: labels, Multiple: field.Multiple, AllowCustom: field.AllowCustom}
	}
	return questioncontract.New(fields)
}

func (q Question) Answered() bool { return q.Answers != nil }

func (q Question) Equal(other Question) bool {
	if len(q.Fields) != len(other.Fields) || q.Answered() != other.Answered() {
		return false
	}
	for index, field := range q.Fields {
		candidate := other.Fields[index]
		if field.Prompt != candidate.Prompt ||
			field.Header != candidate.Header ||
			field.Kind != candidate.Kind ||
			field.Multiple != candidate.Multiple ||
			field.AllowCustom != candidate.AllowCustom ||
			!slices.Equal(field.Options, candidate.Options) {
			return false
		}
	}
	if !q.Answered() {
		return true
	}
	return slices.EqualFunc(q.Answers, other.Answers, func(left, right []string) bool {
		return slices.Equal(left, right)
	})
}

func (field QuestionField) validate(index int) error {
	if strings.TrimSpace(field.Prompt) == "" {
		return fmt.Errorf("question field %d prompt is required", index)
	}
	if utf8.RuneCountInString(field.Header) > MaximumQuestionHeaderCharacters {
		return fmt.Errorf(
			"question field %d header must be at most %d characters",
			index,
			MaximumQuestionHeaderCharacters,
		)
	}
	return nil
}
