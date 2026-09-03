package agent

import (
	"errors"
	"fmt"
	"mime"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"
)

// Run is the lifecycle projection needed by the CLI. ActiveSegmentID exists
// exactly while Status is [protocol.RunStatusRunning].
type Run struct {
	ID              string
	SessionID       string
	Lineage         RunLineage
	Provider        string
	Model           string
	ReasoningEffort string
	Status          protocol.RunStatus
	ActiveSegmentID string
	CreatedAt       time.Time
	FinishedAt      time.Time
	Limits          RunLimits
	ContextTokens   int64
	Outcome         Outcome
	Usage           Usage
	ProtocolProfile *protocol.RunProtocolProfile
}

type runLineageKind uint8

const (
	rootRunLineage runLineageKind = iota + 1
	childRunLineage
)

// RunLineage explicitly identifies either a root run or a child beneath the
// tool block that spawned it. Its zero value is invalid.
type RunLineage struct {
	kind             runLineageKind
	spawnedByBlockID string
	parentRunID      string
	rootRunID        string
}

// RootRunLineage constructs explicit root-run lineage.
func RootRunLineage() RunLineage { return RunLineage{kind: rootRunLineage} }

// NewChildRunLineage constructs and validates a child-run identity tuple.
func NewChildRunLineage(runID, spawnedByBlockID, parentRunID, rootRunID string) (RunLineage, error) {
	lineage := RunLineage{
		kind: childRunLineage, spawnedByBlockID: spawnedByBlockID,
		parentRunID: parentRunID, rootRunID: rootRunID,
	}
	if err := lineage.validate(runID); err != nil {
		return RunLineage{}, err
	}
	return lineage, nil
}

func (r RunLineage) IsRoot() bool {
	return r.kind == rootRunLineage
}

func (r RunLineage) SpawnedByBlockID() string { return r.spawnedByBlockID }

func (r RunLineage) ParentRunID() string { return r.parentRunID }

func (r RunLineage) RootRunID() string { return r.rootRunID }

func (r Run) Clone() Run {
	r.Outcome = r.Outcome.Clone()
	r.Usage = r.Usage.Clone()
	r.ProtocolProfile = cloneRunProtocolProfile(r.ProtocolProfile)
	return r
}

// Equal reports whether two run projections carry the same lifecycle fact.
func (r Run) Equal(other Run) bool {
	return r.ID == other.ID && r.SessionID == other.SessionID && r.Lineage == other.Lineage && r.Provider == other.Provider &&
		r.Model == other.Model && r.ReasoningEffort == other.ReasoningEffort &&
		r.Status == other.Status && r.ActiveSegmentID == other.ActiveSegmentID &&
		r.CreatedAt.Equal(other.CreatedAt) && r.FinishedAt.Equal(other.FinishedAt) &&
		r.Limits == other.Limits && r.ContextTokens == other.ContextTokens &&
		r.Outcome.Equal(other.Outcome) && r.Usage.Equal(other.Usage) &&
		equalRunProtocolProfiles(r.ProtocolProfile, other.ProtocolProfile)
}

func cloneRunProtocolProfile(profile *protocol.RunProtocolProfile) *protocol.RunProtocolProfile {
	if profile == nil {
		return nil
	}
	cloned := *profile
	cloned.RequiredFeatures = slices.Clone(profile.RequiredFeatures)
	cloned.InterruptTypes = slices.Clone(profile.InterruptTypes)
	return &cloned
}

func equalRunProtocolProfiles(left, right *protocol.RunProtocolProfile) bool {
	if (left == nil) != (right == nil) {
		return false
	}
	return left == nil || slices.Equal(left.RequiredFeatures, right.RequiredFeatures) &&
		slices.Equal(left.InterruptTypes, right.InterruptTypes)
}

type Message struct {
	Text        string
	Attachments []Attachment
}

// SteerRun injects an instruction only into the exact running segment the user
// is currently observing. A stale segment must be rejected, never retargeted.
type SteerRun struct {
	CommandID CommandID
	RunID     string
	SegmentID string
	Message   Message
}

func (s SteerRun) Clone() SteerRun {
	s.Message = s.Message.Clone()
	return s
}

func (s SteerRun) Equal(other SteerRun) bool {
	return s.CommandID == other.CommandID && s.RunID == other.RunID &&
		s.SegmentID == other.SegmentID && s.Message.Equal(other.Message)
}

func (m Message) Clone() Message {
	m.Text = strings.Clone(m.Text)
	m.Attachments = slices.Clone(m.Attachments)
	return m
}

// Equal reports whether two messages have the same complete authoring value.
// Attachment metadata participates because restored drafts and history must not
// silently retain a stale projection for an otherwise identical attachment ID.
func (m Message) Equal(other Message) bool {
	return m.Text == other.Text && slices.Equal(m.Attachments, other.Attachments)
}

// HasText reports whether the authored text contains semantic content. It does
// not normalize Text: leading indentation and trailing newlines belong to the
// user's prompt and must survive delivery unchanged.
func (m Message) HasText() bool { return strings.TrimSpace(m.Text) != "" }

// IsEmpty reports whether the message has neither semantic text nor an
// attachment. Whitespace next to attachments is not a separate text block.
func (m Message) IsEmpty() bool { return !m.HasText() && len(m.Attachments) == 0 }

type Attachment struct {
	ID       string
	Kind     protocol.ContentBlockType
	Name     string
	Path     string
	MimeType string
	Size     int64
}

func (a Attachment) Validate() error {
	var problems []error
	if strings.TrimSpace(a.ID) == "" {
		problems = append(problems, errors.New("id is empty"))
	}
	if !slices.Contains([]protocol.ContentBlockType{protocol.ContentBlockImage, protocol.ContentBlockText}, a.Kind) {
		problems = append(problems, fmt.Errorf("kind %q is invalid", a.Kind))
	}
	if a.Kind == protocol.ContentBlockImage {
		mediaType, _, err := mime.ParseMediaType(a.MimeType)
		if err != nil || !strings.HasPrefix(mediaType, "image/") {
			problems = append(problems, fmt.Errorf("image MIME %q is invalid", a.MimeType))
		}
	}
	if strings.TrimSpace(a.Name) == "" {
		problems = append(problems, errors.New("name is empty"))
	}
	if a.Size < 0 {
		problems = append(problems, errors.New("size is negative"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("attachment: %w", err)
	}
	return nil
}

type RunOptions struct {
	Provider        string
	Model           string
	ReasoningEffort string
	Limits          RunLimits
	Generation      protocol.GenerationParams
}

func (r RunOptions) Clone() RunOptions {
	if r.Generation.Temperature != nil {
		r.Generation.Temperature = new(*r.Generation.Temperature)
	}
	if r.Generation.MaxTokens != nil {
		r.Generation.MaxTokens = new(*r.Generation.MaxTokens)
	}
	if r.Generation.TopP != nil {
		r.Generation.TopP = new(*r.Generation.TopP)
	}
	r.Generation.Stop = slices.Clone(r.Generation.Stop)
	return r
}

// Equal reports whether two run starts would carry the same complete execution
// configuration. Optional generation values retain nil-vs-zero semantics.
func (r RunOptions) Equal(other RunOptions) bool {
	return r.Provider == other.Provider && r.Model == other.Model && r.ReasoningEffort == other.ReasoningEffort &&
		r.Limits == other.Limits &&
		equalOptional(r.Generation.Temperature, other.Generation.Temperature) &&
		equalOptional(r.Generation.MaxTokens, other.Generation.MaxTokens) &&
		equalOptional(r.Generation.TopP, other.Generation.TopP) &&
		slices.Equal(r.Generation.Stop, other.Generation.Stop)
}

// Interaction is a closed interrupt payload.
type Interaction interface{ isInteraction() }

type Approval struct {
	RunID        string
	ItemID       string
	Title        string
	Detail       string
	Tool         *ToolCall
	Diff         string
	Risk         protocol.ApprovalRisk
	RuleHint     string
	Rememberable bool
}

type Question struct {
	RunID  string
	ItemID string
	Title  string
	Detail string
	Fields []QuestionField
	// Answers is nil while the question is pending. Once the runtime accepts a
	// response, it preserves one values slice per field as a transcript fact.
	Answers [][]string
}

type QuestionKind string

const (
	QuestionText   QuestionKind = "text"
	QuestionSingle QuestionKind = "single"
	QuestionMulti  QuestionKind = "multi"
)

type QuestionField struct {
	Prompt      string
	Header      string
	Kind        QuestionKind
	AllowCustom bool
	Options     []QuestionOption
}

type QuestionOption struct {
	Label       string
	Description string
	Preview     string
}

func (Approval) isInteraction() {}
func (Question) isInteraction() {}

type Answer interface{ isAnswer() }

type ApprovalAnswer struct {
	Decision         protocol.ApprovalDecision
	Remember         protocol.RememberScopeKind
	Reason           string
	ArgumentOverride *ToolArgumentOverride
}

// QuestionAnswer preserves the field order from Question.Fields, matching the
// runtime's ordered answer matrix.
type QuestionAnswer struct {
	Values [][]string
}

func (ApprovalAnswer) isAnswer() {}
func (QuestionAnswer) isAnswer() {}

type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	// CostUSD is nil when the runtime cannot price the usage. A present zero is
	// distinct: it is known, priced usage whose current cost is zero.
	CostUSD *float64
	// ByModel retains the runtime's cumulative attribution without coupling the
	// conversation domain to provider-specific model registries.
	ByModel map[string]ModelUsage
	Steps   int
	// Duration is active execution time; human-interrupt waiting is excluded.
	Duration time.Duration
}

// ModelUsage is one model's cumulative metering slice within a run.
type ModelUsage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	CostUSD          *float64
}

func (u Usage) Clone() Usage {
	if u.CostUSD != nil {
		u.CostUSD = new(*u.CostUSD)
	}
	if u.ByModel != nil {
		cloned := make(map[string]ModelUsage, len(u.ByModel))
		for model, usage := range u.ByModel {
			cloned[model] = usage.Clone()
		}
		u.ByModel = cloned
	}
	return u
}

// Equal preserves the distinction between unknown cost and a known zero cost.
func (u Usage) Equal(other Usage) bool {
	if u.InputTokens != other.InputTokens || u.OutputTokens != other.OutputTokens ||
		u.CacheReadTokens != other.CacheReadTokens || u.CacheWriteTokens != other.CacheWriteTokens ||
		u.ReasoningTokens != other.ReasoningTokens || u.Steps != other.Steps || u.Duration != other.Duration ||
		(u.CostUSD == nil) != (other.CostUSD == nil) {
		return false
	}
	if u.CostUSD != nil && *u.CostUSD != *other.CostUSD {
		return false
	}
	if len(u.ByModel) != len(other.ByModel) {
		return false
	}
	for model, usage := range u.ByModel {
		otherUsage, exists := other.ByModel[model]
		if !exists || !usage.Equal(otherUsage) {
			return false
		}
	}
	return true
}

// Empty reports whether the usage projection carries no metering fact.
func (u Usage) Empty() bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 && u.CacheReadTokens == 0 &&
		u.CacheWriteTokens == 0 && u.ReasoningTokens == 0 && u.CostUSD == nil &&
		len(u.ByModel) == 0 && u.Steps == 0 && u.Duration == 0
}

func (m ModelUsage) Clone() ModelUsage {
	if m.CostUSD != nil {
		m.CostUSD = new(*m.CostUSD)
	}
	return m
}

func (m ModelUsage) Equal(other ModelUsage) bool {
	return m.InputTokens == other.InputTokens && m.OutputTokens == other.OutputTokens &&
		m.CacheReadTokens == other.CacheReadTokens && m.CacheWriteTokens == other.CacheWriteTokens &&
		m.ReasoningTokens == other.ReasoningTokens && (m.CostUSD == nil) == (other.CostUSD == nil) &&
		(m.CostUSD == nil || *m.CostUSD == *other.CostUSD)
}
