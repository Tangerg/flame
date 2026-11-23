package protocol

import (
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
)

//flame:enum
type ItemScopeType string

const (
	ItemScopeSession ItemScopeType = "session"
	ItemScopeRun     ItemScopeType = "run"
)

type ItemListScope struct {
	Type               ItemScopeType `json:"type"`
	SessionID          string        `json:"sessionId,omitempty"`
	RunID              string        `json:"runId,omitempty"`
	IncludeDescendants bool          `json:"includeDescendants,omitzero"`
}

//flame:enum
type ItemOrder string

const (
	ItemOrderAsc  ItemOrder = "asc"
	ItemOrderDesc ItemOrder = "desc"
)

type ListItemsRequest struct {
	Scope ItemListScope `json:"scope"`
	Order ItemOrder     `json:"order,omitempty"`
	PageQuery
}

type ListItemsResponse struct {
	Page[Item]
	Runs []RunSummary `json:"runs"`
}

//flame:enum
type ItemStatus string

const (
	ItemStatusRunning    ItemStatus = "running"
	ItemStatusCompleted  ItemStatus = "completed"
	ItemStatusIncomplete ItemStatus = "incomplete"
)

//flame:enum
type ItemType string

const (
	ItemTypeUserMessage  ItemType = "userMessage"
	ItemTypeAgentMessage ItemType = "agentMessage"
	ItemTypeReasoning    ItemType = "reasoning"
	ItemTypeQuestion     ItemType = "question"
	ItemTypeToolCall     ItemType = "toolCall"
	ItemTypeCompaction   ItemType = "compaction"
)

//flame:enum
type MessagePhase string

const (
	MessagePhaseCommentary  MessagePhase = "commentary"
	MessagePhaseFinalAnswer MessagePhase = "finalAnswer"
)

//flame:enum
type SafetyClass string

const (
	SafetyClassSafe    SafetyClass = "safe"
	SafetyClassWrite   SafetyClass = "write"
	SafetyClassExec    SafetyClass = "exec"
	SafetyClassNetwork SafetyClass = "network"
)

//flame:enum
type ContentBlockType string

const (
	ContentBlockText  ContentBlockType = "text"
	ContentBlockImage ContentBlockType = "image"
)

//flame:enum
type QuestionFieldType string

const (
	QuestionFieldText   QuestionFieldType = QuestionFieldType(questioncontract.Text)
	QuestionFieldChoice QuestionFieldType = QuestionFieldType(questioncontract.Choice)
)

func (QuestionFieldType) Values() []string { return questioncontract.Kind("").Values() }

//flame:enum
type DiffRowType string

const (
	DiffRowHunk    DiffRowType = "hunk"
	DiffRowContext DiffRowType = "context"
	DiffRowAdded   DiffRowType = "added"
	DiffRowDeleted DiffRowType = "deleted"
)

type Item struct {
	ID             string     `json:"id"`
	RunID          string     `json:"runId"`
	Status         ItemStatus `json:"status"`
	CreatedAt      time.Time  `json:"createdAt,omitzero"`
	Type           ItemType   `json:"type"`
	StartedAt      time.Time  `json:"startedAt,omitzero"`
	FinishedAt     time.Time  `json:"finishedAt,omitzero"`
	DurationMillis *int64     `json:"durationMillis,omitzero"`

	Content          []ContentBlock   `json:"content,omitempty"`
	Phase            MessagePhase     `json:"phase,omitempty"`
	Text             string           `json:"text,omitempty"`
	Redacted         bool             `json:"redacted,omitzero"`
	Question         *Question        `json:"question,omitzero"`
	Tool             *ToolInvocation  `json:"tool,omitzero"`
	SafetyClass      SafetyClass      `json:"safetyClass,omitempty"`
	ApprovalDecision ApprovalDecision `json:"approvalDecision,omitempty"`
	Error            *ProblemData     `json:"error,omitzero"`
	Summary          string           `json:"summary,omitempty"`
	DroppedMessages  int              `json:"droppedMessages,omitzero"`
}

type ContentBlock struct {
	Type ContentBlockType `json:"type"`
	Text string           `json:"text,omitempty"`
	Mime string           `json:"mime,omitempty"`
	Data string           `json:"data,omitempty"`
}

type Question struct {
	Fields  []QuestionField `json:"fields"`
	Answers [][]string      `json:"answers,omitempty"`
}

type QuestionField struct {
	Prompt      string            `json:"prompt"`
	Header      string            `json:"header,omitempty"`
	Type        QuestionFieldType `json:"type"`
	Options     []QuestionOption  `json:"options,omitempty"`
	Multiple    bool              `json:"multiple,omitzero"`
	AllowCustom bool              `json:"allowCustom,omitzero"`
}

type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Preview     string `json:"preview,omitempty"`
}

type ToolInvocation struct {
	Source        *RecordedToolSource   `json:"source,omitzero"`
	ArgumentsText string                `json:"argumentsText,omitempty"`
	Name          string                `json:"name"`
	Category      builtin.ToolCategory  `json:"category,omitempty"`
	ResultShape   extension.ResultShape `json:"resultShape,omitempty"`
	Arguments     map[string]any        `json:"arguments"`
	Result        any                   `json:"result,omitzero"`
}

type RecordedToolSource struct {
	Ref       ToolRef `json:"ref"`
	ModelName string  `json:"modelName"`
}

type DiffRow struct {
	Type      DiffRowType `json:"type"`
	Text      string      `json:"text,omitempty"`
	LeftLine  int         `json:"leftLine,omitzero"`
	RightLine int         `json:"rightLine,omitzero"`
	Code      *string     `json:"code,omitzero"`
	NoNewline bool        `json:"noNewline,omitzero"`
}

type ModelUsage struct {
	InputTokens      int64    `json:"inputTokens,omitzero"`
	OutputTokens     int64    `json:"outputTokens,omitzero"`
	CacheReadTokens  int64    `json:"cacheReadTokens,omitzero"`
	CacheWriteTokens int64    `json:"cacheWriteTokens,omitzero"`
	ReasoningTokens  int64    `json:"reasoningTokens,omitzero"`
	CostUSD          *float64 `json:"costUsd,omitzero"`
}

type Usage struct {
	ModelUsage
	ByModel map[string]ModelUsage `json:"byModel,omitempty"`
}
