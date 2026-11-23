package conversation

import (
	"errors"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type RunReference struct {
	SessionID string
	RunID     string
}

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
	ContextTokens   int64
	Outcome         Outcome
	Usage           Usage
	Capabilities    *protocol.RunCapabilities
}

type runLineageKind uint8

const (
	rootRunLineage runLineageKind = iota + 1
	childRunLineage
)

type RunLineage struct {
	kind             runLineageKind
	spawnedByBlockID string
	parentRunID      string
	rootRunID        string
}

func RootRunLineage() RunLineage { return RunLineage{kind: rootRunLineage} }

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
	r.Capabilities = cloneRunCapabilities(r.Capabilities)
	return r
}

func (r Run) Equal(other Run) bool {
	return r.ID == other.ID && r.SessionID == other.SessionID && r.Lineage == other.Lineage && r.Provider == other.Provider &&
		r.Model == other.Model && r.ReasoningEffort == other.ReasoningEffort &&
		r.Status == other.Status && r.ActiveSegmentID == other.ActiveSegmentID &&
		r.CreatedAt.Equal(other.CreatedAt) && r.FinishedAt.Equal(other.FinishedAt) &&
		r.ContextTokens == other.ContextTokens &&
		r.Outcome.Equal(other.Outcome) && r.Usage.Equal(other.Usage) &&
		equalRunCapabilitiess(r.Capabilities, other.Capabilities)
}

func cloneRunCapabilities(profile *protocol.RunCapabilities) *protocol.RunCapabilities {
	if profile == nil {
		return nil
	}
	cloned := *profile
	cloned.RequiredFeatures = slices.Clone(profile.RequiredFeatures)
	cloned.InterruptTypes = slices.Clone(profile.InterruptTypes)
	return &cloned
}

func equalRunCapabilitiess(left, right *protocol.RunCapabilities) bool {
	if (left == nil) != (right == nil) {
		return false
	}
	return left == nil || slices.Equal(left.RequiredFeatures, right.RequiredFeatures) &&
		slices.Equal(left.InterruptTypes, right.InterruptTypes)
}

var ErrSteerReceiptUnavailable = errors.New("steer acceptance receipt is unavailable")

type Interrupt interface{ isInterrupt() }

type Approval struct {
	RunID        string                `json:"runId"`
	ItemID       string                `json:"itemId"`
	Title        string                `json:"title"`
	Detail       string                `json:"detail"`
	Tool         *ToolCall             `json:"tool"`
	Risk         protocol.ApprovalRisk `json:"risk"`
	Rememberable bool                  `json:"rememberable"`
}

type Question struct {
	RunID   string          `json:"runId"`
	ItemID  string          `json:"itemId"`
	Title   string          `json:"title"`
	Fields  []QuestionField `json:"fields"`
	Answers [][]string      `json:"answers"`
}

type QuestionKind string

const (
	QuestionText   QuestionKind = "text"
	QuestionSingle QuestionKind = "single"
	QuestionMulti  QuestionKind = "multi"
)

type QuestionField struct {
	Prompt      string                    `json:"prompt"`
	Header      string                    `json:"header"`
	Kind        QuestionKind              `json:"kind"`
	AllowCustom bool                      `json:"allowCustom"`
	Options     []protocol.QuestionOption `json:"options"`
}

func (Approval) isInterrupt() {}

func (Question) isInterrupt() {}

type Answer interface{ isAnswer() }

type ApprovalAnswer struct {
	Decision         protocol.ApprovalDecision  `json:"decision"`
	Remember         protocol.RememberScopeKind `json:"remember"`
	Reason           string                     `json:"reason"`
	ArgumentOverride *ToolArgumentOverride      `json:"argumentOverride"`
}

type QuestionAnswer struct {
	Values [][]string `json:"values"`
}

func (ApprovalAnswer) isAnswer() {}

func (QuestionAnswer) isAnswer() {}

type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	// Nil means the runtime cannot price the usage; a present zero is known, priced
	// usage.
	CostUSD  *float64
	ByModel  map[string]protocol.ModelUsage
	Steps    int
	Duration time.Duration
}

func (u Usage) Clone() Usage {
	if u.CostUSD != nil {
		u.CostUSD = new(*u.CostUSD)
	}
	if u.ByModel != nil {
		cloned := make(map[string]protocol.ModelUsage, len(u.ByModel))
		for model, usage := range u.ByModel {
			cloned[model] = cloneProtocolModelUsage(usage)
		}
		u.ByModel = cloned
	}
	return u
}

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
		if !exists || !equalProtocolModelUsage(usage, otherUsage) {
			return false
		}
	}
	return true
}

func (u Usage) Empty() bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 && u.CacheReadTokens == 0 &&
		u.CacheWriteTokens == 0 && u.ReasoningTokens == 0 && u.CostUSD == nil &&
		len(u.ByModel) == 0 && u.Steps == 0 && u.Duration == 0
}

func cloneProtocolModelUsage(usage protocol.ModelUsage) protocol.ModelUsage {
	if usage.CostUSD != nil {
		usage.CostUSD = new(*usage.CostUSD)
	}
	return usage
}

func equalProtocolModelUsage(left, right protocol.ModelUsage) bool {
	return left.InputTokens == right.InputTokens && left.OutputTokens == right.OutputTokens &&
		left.CacheReadTokens == right.CacheReadTokens && left.CacheWriteTokens == right.CacheWriteTokens &&
		left.ReasoningTokens == right.ReasoningTokens && (left.CostUSD == nil) == (right.CostUSD == nil) &&
		(left.CostUSD == nil || *left.CostUSD == *right.CostUSD)
}
