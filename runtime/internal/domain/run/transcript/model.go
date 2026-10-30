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

// ErrIdentityConflict reports an attempt to reuse a durable transcript identity
// for a different owner. Item ids are bound to one Session+Run and Run ids to
// one Session for their entire lifetime; persistence must never re-parent them.
var ErrIdentityConflict = errors.New("transcript: identity conflict")

type ItemStatus string

const (
	ItemRunning    ItemStatus = "running"
	ItemCompleted  ItemStatus = "completed"
	ItemIncomplete ItemStatus = "incomplete"
)

// Valid reports whether i belongs to the durable Item lifecycle vocabulary.
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

// MessagePhase names the semantic role of one AgentMessage in a model turn.
// Commentary is progress or a preamble before more work; FinalAnswer is the
// terminal response a completed Run leaves with the user. The phase is authored
// at the model-call boundary and survives every transcript representation.
type MessagePhase string

const (
	MessagePhaseNone   MessagePhase = ""
	MessageCommentary  MessagePhase = "commentary"
	MessageFinalAnswer MessagePhase = "finalAnswer"
)

// Valid reports whether m is one of the two authored AgentMessage roles.
func (m MessagePhase) Valid() bool {
	return m == MessageCommentary || m == MessageFinalAnswer
}

// SequencedItem pairs a history Item with its position in the session's durable
// append order — the total order a paged read continues along, and the only one
// that is exact: occurrence timestamps tie, and an imported transcript can carry
// backdated ones.
//
// The position sits beside the Item rather than inside it because the store
// assigns it when the Item lands: an Item on its way to being appended has no
// position, so a field for one inside the aggregate would be a zero every writer
// had to remember to ignore.
type SequencedItem struct {
	Sequence int64
	Item     Item
}

// SequenceOrder is the direction a paged read walks the durable sequence. Both
// directions are the SAME total order read from opposite ends — never a different
// sort — so a page is exact either way and the two cannot disagree about which item
// comes first.
type SequenceOrder string

const (
	// OldestFirst replays the session the way it happened, which is what folding it
	// back into state requires.
	OldestFirst SequenceOrder = "oldest"
	// NewestFirst reaches the tail without walking everything before it, which is
	// what showing a long session's last screen requires.
	NewestFirst SequenceOrder = "newest"
)

// Valid reports whether s names one of the two directions through the durable
// transcript sequence.
func (s SequenceOrder) Valid() bool {
	return s == OldestFirst || s == NewestFirst
}

// Validate rejects a direction that cannot define cursor and SQL ordering.
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

// Clone returns an ownership-isolated content value.
func (c ContentBlock) Equal(other ContentBlock) bool {
	return c.Kind == other.Kind && c.Text == other.Text && c.MediaType == other.MediaType && bytes.Equal(c.Bytes, other.Bytes)
}

func (c ContentBlock) Clone() ContentBlock {
	c.Bytes = slices.Clone(c.Bytes)
	return c
}

// CloneContent returns an ownership-isolated sequence of content blocks.
func CloneContent(blocks []ContentBlock) []ContentBlock {
	cloned := make([]ContentBlock, len(blocks))
	for index, block := range blocks {
		cloned[index] = block.Clone()
	}
	return cloned
}

type Question struct {
	Fields []QuestionField
	// Answers is the ordered response accepted by the Runtime, when one exists.
	// A nil value means no response was accepted; the root-owned Pending set is
	// still the sole authority on whether the question is currently open.
	Answers [][]string
}

// Headers are compact presentation chips; Prompt retains the complete question.
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
	// ArgumentsText retains unexecuted input verbatim. When present, Arguments is empty.
	ArgumentsText string
	Name          string
	Arguments     tool.Arguments
	Result        *tool.Result
	Offload       *ToolResultRef
}

// Equal compares the semantic Tool invocation value without exposing the
// canonical storage representation of arguments or results to callers.
func (t ToolInvocation) Equal(other ToolInvocation) bool {
	if t.Name != other.Name || t.ArgumentsText != other.ArgumentsText || !t.Arguments.Equal(other.Arguments) {
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

// Interrupt is one thing a person has to answer before execution continues.
type Interrupt struct {
	ItemID string
	// ItemOccurredAt preserves the identity timestamp of the Item this interrupt
	// references. An approval eventually settles that same ToolCall; a Question is
	// already a complete prompt fact. Neither path may manufacture a new occurrence
	// when the Run resumes.
	ItemOccurredAt time.Time
	// RunID is the Run that RAISED this interrupt — not the Run that owns the set
	// it belongs to. The two are the same only while a tree is a single Run: a set
	// is owned by the root and consumed as a whole, while each interrupt was raised
	// somewhere in that tree and is answered in the context of the Run that asked.
	//
	// It is recorded rather than derived, because the derivation stops working
	// exactly when it starts to matter.
	RunID    string
	Kind     run.InterruptKind
	Approval *Approval
	Question *Question
}

// Validate reports whether an interrupt carries exactly the one payload its Kind
// names, with a valid item and run identity. An open question that already holds
// an accepted answer is no longer open, so it is rejected here rather than resumed.
func (i Interrupt) Validate() error {
	if err := identity.ValidateItem(i.ItemID); err != nil {
		return fmt.Errorf("pending interrupt: %w", err)
	}
	if i.ItemOccurredAt.IsZero() {
		return errors.New("item occurrence time is required")
	}
	if err := identity.ValidateRun(i.RunID); err != nil {
		return fmt.Errorf("pending interrupt: %w", err)
	}
	switch i.Kind {
	case run.InterruptApproval:
		if i.Approval == nil || i.Question != nil {
			return errors.New("approval interrupt requires only an approval payload")
		}
		return i.Approval.Validate()
	case run.InterruptQuestion:
		if i.Question == nil || i.Approval != nil {
			return errors.New("question interrupt requires only a question payload")
		}
		if err := i.Question.Validate(); err != nil {
			return err
		}
		if i.Question.Answered() {
			return errors.New("open question interrupt already carries an accepted answer")
		}
		return nil
	default:
		return fmt.Errorf("unknown interrupt kind %q", i.Kind)
	}
}

type Approval struct {
	Tool         ToolInvocation
	Risk         tool.RiskLevel
	Reason       string
	Rememberable bool
}

// Validate reports whether an approval describes one pending tool invocation.
// A result or offload reference would mean the invocation already ran, while a
// missing risk cannot support an informed decision.
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

// Validate reports whether the content block has the shape required by its kind.
func (c ContentBlock) Validate() error {
	switch c.Kind {
	case TextContent:
		if strings.TrimSpace(c.Text) == "" {
			return errors.New("text content requires text")
		}
		if c.MediaType != "" || len(c.Bytes) != 0 {
			return errors.New("text content cannot carry media")
		}
	case ImageContent:
		if !strings.HasPrefix(c.MediaType, "image/") || len(c.Bytes) == 0 {
			return errors.New("image content requires an image media type and bytes")
		}
		if c.Text != "" {
			return errors.New("image content cannot carry text")
		}
	default:
		return fmt.Errorf("unknown content kind %q", c.Kind)
	}
	return nil
}

// Validate reports whether the question can be rendered and answered unambiguously.
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

// AnswerContract projects this question's admitted definition into the shared
// Runtime contract without carrying its presentation or accepted-answer state.
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

// Answered reports whether the Runtime accepted an answer for this question.
// It does not claim that an unanswered question is still open; Pending owns
// that separate lifecycle fact.
func (q Question) Answered() bool { return q.Answers != nil }

// Equal compares the semantic question value. Empty option or answer slices
// are equivalent regardless of backing representation; the top-level nil
// Answers value remains distinct because it means no response was accepted.
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
