package protocol

import (
	"encoding/json/jsontext"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
)

//flame:enum
type SessionStatus string

const (
	SessionStatusRunning SessionStatus = "running"
	SessionStatusWaiting SessionStatus = "waiting"
	SessionStatusIdle    SessionStatus = "idle"
)

type Session struct {
	ID              string        `json:"id"`
	Title           string        `json:"title"`
	Status          SessionStatus `json:"status"`
	Provider        string        `json:"provider"`
	Model           string        `json:"model"`
	ReasoningEffort string        `json:"reasoningEffort,omitempty"`
	Workspace       WorkspaceInfo `json:"workspace"`
	CreatedAt       time.Time     `json:"createdAt,omitzero"`
	UpdatedAt       time.Time     `json:"updatedAt,omitzero"`
	Favorite        bool          `json:"favorite,omitzero"`
	Isolated        bool          `json:"isolated,omitzero"`
	Revision        uint64        `json:"revision"`
}

type ListSessionsRequest struct {
	PageQuery
	Search    string        `json:"search,omitempty"`
	Workspace *WorkspaceRef `json:"workspace,omitzero"`
}

type GetSessionRequest struct {
	SessionID string `json:"sessionId"`
}

type GetSessionSnapshotRequest struct {
	SessionID          string `json:"sessionId"`
	IncludeDescendants bool   `json:"includeDescendants,omitzero"`
}

type SessionSnapshot struct {
	SteerInputs []SteerInput          `json:"steerInputs,omitempty"`
	Session     Session               `json:"session"`
	Items       []Item                `json:"items"`
	Runs        []RunRef              `json:"runs"`
	Interrupts  []PendingInterruptSet `json:"interrupts"`
	Plan        *Plan                 `json:"plan,omitzero"`
	Goal        *Goal                 `json:"goal,omitzero"`
}

type DeleteSessionRequest struct {
	SessionID string `json:"sessionId"`
}

type CreateSessionRequest struct {
	Workspace *WorkspaceRef `json:"workspace,omitzero"`
	Title     string        `json:"title,omitempty"`
}

type UpdateSessionRequest struct {
	SessionID        string        `json:"sessionId"`
	ExpectedRevision uint64        `json:"expectedRevision"`
	Title            *string       `json:"title,omitzero"`
	Workspace        *WorkspaceRef `json:"workspace,omitzero"`
	Provider         *string       `json:"provider,omitzero"`
	Model            *string       `json:"model,omitzero"`
	ReasoningEffort  *string       `json:"reasoningEffort,omitzero"`
	Favorite         *bool         `json:"favorite,omitzero"`
	Isolated         *bool         `json:"isolated,omitzero"`
}

type ForkSessionRequest struct {
	SessionID string `json:"sessionId"`
	FromRunID string `json:"fromRunId,omitempty"`
	Title     string `json:"title,omitempty"`
}

type RollbackSessionRequest struct {
	SessionID   string      `json:"sessionId"`
	ToRunID     string      `json:"toRunId,omitempty"`
	RestoreType RestoreType `json:"restoreType,omitempty"`
}

//flame:enum
type RestoreType string

const (
	RestoreHistory RestoreType = "history"
	RestoreFiles   RestoreType = "files"
	RestoreBoth    RestoreType = "both"
)

type RollbackSessionResponse struct {
	Session     *Session     `json:"session"`
	DroppedRuns []DroppedRun `json:"droppedRuns"`
}

type DroppedRun struct {
	Run       RunSummary     `json:"run"`
	UserInput []ContentBlock `json:"userInput,omitempty"`
}

//flame:enum
type ExportFormat string

const (
	ExportFormatMarkdown ExportFormat = "md"
	ExportFormatJSON     ExportFormat = "json"
)

type ExportSessionRequest struct {
	SessionID string       `json:"sessionId"`
	Format    ExportFormat `json:"format,omitempty"`
}

type ExportSessionResponse struct {
	Format   ExportFormat     `json:"format"`
	Artifact *SessionArtifact `json:"artifact,omitzero"`
	Markdown string           `json:"markdown,omitempty"`
}

const SessionArtifactVersion = 32

const MaximumSessionArtifactBytes = 64 << 20

type SessionArtifact struct {
	Version     int                  `json:"version"`
	Session     ArtifactSession      `json:"session"`
	Messages    []jsontext.Value     `json:"messages"`
	Runs        []ArtifactRun        `json:"runs"`
	Items       []ArtifactItem       `json:"items"`
	ToolResults []ArtifactToolResult `json:"toolResults"`
	// Revision and updatedAt are the source runtime's ordering tokens; carrying them
	// would let an imported value claim a revision the target never issued.
	Plan       []PlanStep                    `json:"plan,omitzero"`
	PlanSource *extension.ContributionSource `json:"planSource,omitzero"`
}

type ArtifactSession struct {
	ID              string       `json:"id"`
	Title           string       `json:"title"`
	Workspace       WorkspaceRef `json:"workspace"`
	Provider        string       `json:"provider"`
	Model           string       `json:"model"`
	ReasoningEffort string       `json:"reasoningEffort,omitempty"`
	CreatedAt       time.Time    `json:"createdAt,omitzero"`
	UpdatedAt       time.Time    `json:"updatedAt,omitzero"`
	Favorite        bool         `json:"favorite,omitzero"`
}

type ArtifactRun struct {
	ID              string           `json:"id"`
	SessionID       string           `json:"sessionId"`
	SpawnedByItemID string           `json:"spawnedByItemId,omitempty"`
	ParentRunID     string           `json:"parentRunId,omitempty"`
	RootRunID       string           `json:"rootRunId,omitempty"`
	Provider        string           `json:"provider"`
	Model           string           `json:"model"`
	ReasoningEffort string           `json:"reasoningEffort,omitempty"`
	Metrics         RunMetrics       `json:"metrics"`
	ContextTokens   int64            `json:"contextTokens,omitzero"`
	Capabilities    *RunCapabilities `json:"capabilities,omitzero"`
	Outcome         ArtifactOutcome  `json:"outcome"`
	CreatedAt       time.Time        `json:"createdAt,omitzero"`
	FinishedAt      time.Time        `json:"finishedAt,omitzero"`
	UpdatedAt       time.Time        `json:"updatedAt,omitzero"`
	MessageMark     *int             `json:"messageMark,omitzero"`
}

type ArtifactOutcome struct {
	Type              ArtifactOutcomeType `json:"type"`
	Error             *ArtifactProblem    `json:"error,omitzero"`
	Detail            string              `json:"detail,omitempty"`
	Loss              *RunLoss            `json:"loss,omitzero"`
	UnresolvedEffects []UnresolvedEffect  `json:"unresolvedEffects,omitempty"`
}

//flame:enum
type ArtifactOutcomeType string

const (
	ArtifactOutcomeCompleted ArtifactOutcomeType = "completed"
	ArtifactOutcomeTimedOut  ArtifactOutcomeType = "timedOut"
	ArtifactOutcomeFailed    ArtifactOutcomeType = "failed"

	ArtifactOutcomeCanceled ArtifactOutcomeType = "canceled"
	ArtifactOutcomeLost     ArtifactOutcomeType = "lost"
)

type ArtifactItem struct {
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
	Error            *ArtifactProblem `json:"error,omitzero"`
	Summary          string           `json:"summary,omitempty"`
	DroppedMessages  int              `json:"droppedMessages,omitzero"`
}

type ArtifactProblem struct {
	Type              ArtifactProblemType `json:"type"`
	Detail            string              `json:"detail,omitempty"`
	DocURL            string              `json:"docUrl,omitempty"`
	RetryAfterSeconds int                 `json:"retryAfterSeconds,omitzero"`
}

//flame:enum
type ArtifactProblemType string

const (
	ArtifactProblemInternalError       ArtifactProblemType = ProblemInternalError
	ArtifactProblemAgentStuck          ArtifactProblemType = ProblemAgentStuck
	ArtifactProblemRateLimited         ArtifactProblemType = ProblemRateLimited
	ArtifactProblemInvalidCredentials  ArtifactProblemType = ProblemInvalidCredentials
	ArtifactProblemTimeout             ArtifactProblemType = ProblemTimeout
	ArtifactProblemProviderUnavailable ArtifactProblemType = ProblemProviderUnavailable
	ArtifactProblemProviderRejected    ArtifactProblemType = ProblemProviderRejected
	ArtifactProblemDeniedByUser        ArtifactProblemType = ProblemDeniedByUser
	ArtifactProblemToolFailed          ArtifactProblemType = ProblemToolFailed
	ArtifactProblemChildRunCanceled    ArtifactProblemType = ProblemChildRunCanceled
	ArtifactProblemToolCanceled        ArtifactProblemType = ProblemToolCanceled
)

type ArtifactToolResult struct {
	ID        string    `json:"id"`
	ItemID    string    `json:"itemId"`
	Body      string    `json:"body"`
	ModelOnly bool      `json:"modelOnly"`
	CreatedAt time.Time `json:"createdAt,omitzero"`
}

type ImportSessionRequest struct {
	Artifact SessionArtifact `json:"artifact"`
}

type ImportSessionResponse struct {
	Session *Session `json:"session"`
}
