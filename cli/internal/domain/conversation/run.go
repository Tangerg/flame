package conversation

import (
	"errors"
	"slices"
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
		r.ContextTokens == other.ContextTokens &&
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

// ErrSteerReceiptUnavailable marks a successful Runtime call whose acceptance
// receipt is missing. It does not authorize restoring or resending the input.
var ErrSteerReceiptUnavailable = errors.New("steer acceptance receipt is unavailable")

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
	Options     []protocol.QuestionOption
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
	ByModel map[string]protocol.ModelUsage
	Steps   int
	// Duration is active execution time; human-interrupt waiting is excluded.
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
		if !exists || !equalProtocolModelUsage(usage, otherUsage) {
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
