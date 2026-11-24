package protocol

import (
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/runoutcome"
)

//flame:enum
type RunStatus string

const (
	RunStatusRunning  RunStatus = "running"
	RunStatusWaiting  RunStatus = "waiting"
	RunStatusFinished RunStatus = "finished"
)

type RunSummary struct {
	ID              string      `json:"id"`
	SessionID       string      `json:"sessionId"`
	SpawnedByItemID string      `json:"spawnedByItemId,omitempty"`
	ParentRunID     string      `json:"parentRunId,omitempty"`
	RootRunID       string      `json:"rootRunId,omitempty"`
	Model           string      `json:"model"`
	Provider        string      `json:"provider"`
	ReasoningEffort string      `json:"reasoningEffort,omitempty"`
	Status          RunStatus   `json:"status"`
	Outcome         *RunOutcome `json:"outcome,omitzero"`
	CreatedAt       time.Time   `json:"createdAt,omitzero"`
	FinishedAt      time.Time   `json:"finishedAt,omitzero"`
}

type RunRef struct {
	RunSummary
	ActiveSegmentID string           `json:"activeSegmentId,omitempty"`
	Metrics         RunMetrics       `json:"metrics"`
	ContextTokens   int64            `json:"contextTokens,omitzero"`
	Capabilities    *RunCapabilities `json:"capabilities,omitzero"`
}

type RunCapabilityFeature string

const (
	RunCapabilityFeatureChildRuns = RunCapabilityFeature(FeatureChildRuns)
)

type RunCapabilities struct {
	RequiredFeatures []RunCapabilityFeature `json:"requiredFeatures"`
	InterruptTypes   []InterruptType        `json:"interruptTypes"`
}

type RunMetrics struct {
	Usage                *Usage `json:"usage,omitzero"`
	Steps                int    `json:"steps"`
	ActiveDurationMillis int64  `json:"activeDurationMillis"`
}

//flame:enum
type RunOutcomeType string

const (
	OutcomeCompleted RunOutcomeType = RunOutcomeType(runoutcome.Completed)
	OutcomeTimedOut  RunOutcomeType = RunOutcomeType(runoutcome.TimedOut)
	OutcomeFailed    RunOutcomeType = RunOutcomeType(runoutcome.Failed)

	OutcomeCanceled RunOutcomeType = RunOutcomeType(runoutcome.Canceled)
	OutcomeLost     RunOutcomeType = RunOutcomeType(runoutcome.Lost)
)

//flame:enum
type UnresolvedEffectCause string

const (
	UnresolvedAtTermination    UnresolvedEffectCause = "termination"
	UnresolvedAtRuntimeFailure UnresolvedEffectCause = "runtimeFailure"
)

type UnresolvedEffect struct {
	ProcessID string                `json:"processId"`
	EffectID  string                `json:"effectId"`
	Cause     UnresolvedEffectCause `json:"cause"`
	Reason    string                `json:"reason,omitempty"`
	Detail    string                `json:"detail,omitempty"`
	Output    string                `json:"output,omitempty"`
}

type RunOutcome struct {
	UnresolvedEffects []UnresolvedEffect `json:"unresolvedEffects,omitempty"`
	Type              RunOutcomeType     `json:"type"`
	Error             *ProblemData       `json:"error,omitzero"`
	Detail            string             `json:"detail,omitempty"`
	Loss              *RunLoss           `json:"loss,omitzero"`
}

//flame:enum
type LossReason string

const (
	LossRestart                 LossReason = "restart"
	LossIsolatedWorkspace       LossReason = "isolatedWorkspace"
	LossWorkspaceUnavailable    LossReason = "workspaceUnavailable"
	LossOtherBuild              LossReason = "otherBuild"
	LossWaitingStateUnavailable LossReason = "waitingStateUnavailable"
	LossConfigurationChanged    LossReason = "configurationChanged"
	LossUnresolvedEffects       LossReason = "unresolvedEffects"
	LossUnresolvedModelCall     LossReason = "unresolvedModelCall"
)

type RunLoss struct {
	Reason LossReason `json:"reason"`
	Detail string     `json:"detail,omitempty"`
}

type RunSessionOpening struct {
	SessionID string        `json:"sessionId"`
	RunID     string        `json:"runId"`
	Title     string        `json:"title"`
	Workspace *WorkspaceRef `json:"workspace,omitzero"`
	ProjectID string        `json:"projectId,omitempty"`
}

type StartRunRequest struct {
	SessionID       string             `json:"sessionId,omitempty"`
	NewSession      *RunSessionOpening `json:"newSession,omitzero"`
	Input           []ContentBlock     `json:"input"`
	Provider        string             `json:"provider,omitempty"`
	Model           string             `json:"model,omitempty"`
	ReasoningEffort string             `json:"reasoningEffort,omitempty"`
	Params          *GenerationParams  `json:"params,omitzero"`
}

type StartRunResponse struct {
	RunID      string `json:"runId"`
	SegmentID  string `json:"segmentId"`
	UserItemID string `json:"userItemId"`
}

type ResumeRunResponse struct {
	RunID      string  `json:"runId"`
	SegmentID  string  `json:"segmentId"`
	UserItemID *string `json:"userItemId,omitzero"`
}

type GenerationParams struct {
	Temperature *float64 `json:"temperature,omitzero"`
	MaxTokens   *int64   `json:"maxTokens,omitzero"`
	TopP        *float64 `json:"topP,omitzero"`
	Stop        []string `json:"stop,omitempty"`
}

type CancelRunRequest struct {
	RunID  string `json:"runId"`
	Reason string `json:"reason,omitempty"`
}

//flame:enum
type CancelRunResponseType string

const (
	CancelRunRoot  CancelRunResponseType = "root"
	CancelRunChild CancelRunResponseType = "child"
)

type CancelRunResponse struct {
	Type    CancelRunResponseType `json:"type"`
	Run     RunRef                `json:"run"`
	RootRun *RunRef               `json:"rootRun,omitzero"`
}

type SteerRunRequest struct {
	RunID             string         `json:"runId"`
	ExpectedSegmentID string         `json:"expectedSegmentId"`
	Input             []ContentBlock `json:"input"`
}

type SteerRunResponse struct {
	UserItemID string `json:"userItemId"`
}

type ListRunsRequest struct {
	SessionID          string      `json:"sessionId,omitempty"`
	Statuses           []RunStatus `json:"statuses,omitempty"`
	IncludeDescendants bool        `json:"includeDescendants,omitzero"`
	PageQuery
}

type GetRunRequest struct {
	RunID string `json:"runId"`
}

//flame:enum
type RunCommandOperation string

const (
	RunCommandStart  RunCommandOperation = "start"
	RunCommandResume RunCommandOperation = "resume"
	RunCommandSteer  RunCommandOperation = "steer"
)

type GetRunAcceptanceRequest struct {
	Namespace string              `json:"namespace"`
	Key       string              `json:"key"`
	Operation RunCommandOperation `json:"operation"`
	Request   *StartRunRequest    `json:"request,omitzero"`
}

//flame:enum
type RunAcceptanceType string

const (
	RunAcceptanceNotRecorded RunAcceptanceType = "notRecorded"
	RunAcceptanceAccepted    RunAcceptanceType = "accepted"
	RunAcceptanceRetired     RunAcceptanceType = "retired"
)

type RunAcceptance struct {
	CandidateUserItemID string            `json:"candidateUserItemId,omitempty"`
	Type                RunAcceptanceType `json:"type"`
	RunID               string            `json:"runId,omitempty"`
	SegmentID           string            `json:"segmentId,omitempty"`
	SessionID           string            `json:"sessionId,omitempty"`
	UserItemID          string            `json:"userItemId,omitempty"`
	Run                 *RunRef           `json:"run,omitzero"`
	Input               []ContentBlock    `json:"input,omitempty"`
	AppliedItem         *Item             `json:"appliedItem,omitzero"`
}

type SteerInput struct {
	RunID      string         `json:"runId"`
	SegmentID  string         `json:"segmentId"`
	UserItemID string         `json:"userItemId"`
	Input      []ContentBlock `json:"input"`
}

type ListInterruptsRequest struct {
	SessionID string `json:"sessionId,omitempty"`
	RootRunID string `json:"rootRunId,omitempty"`
	PageQuery
}

type ResumeRunRequest struct {
	RunID     string              `json:"runId"`
	Responses []InterruptResponse `json:"responses"`
	Input     []ContentBlock      `json:"input,omitempty"`
}

type SubscribeRunRequest struct {
	Snapshot  bool   `json:"snapshot,omitzero"`
	RunID     string `json:"runId"`
	SegmentID string `json:"segmentId"`
}

type SubscribeRunResponse struct {
	Snapshot  *SessionSnapshot `json:"snapshot,omitzero"`
	RunID     string           `json:"runId"`
	SegmentID string           `json:"segmentId"`
	// Clients may only compare it for equality or send it back unchanged; it is not a
	// watermark.
	HeadEventID *string `json:"headEventId,omitzero"`
}

//flame:enum
type InterruptResponseType string

const (
	InterruptResponseApproval InterruptResponseType = "approval"
	InterruptResponseAnswer   InterruptResponseType = "answer"
)

//flame:enum
type ApprovalDecision string

const (
	ApprovalApprove ApprovalDecision = "approve"
	ApprovalDeny    ApprovalDecision = "deny"
)

type InterruptResponse struct {
	ItemID   string                 `json:"itemId"`
	Response InterruptResponseValue `json:"response"`
}

type InterruptResponseValue struct {
	Type       InterruptResponseType `json:"type"`
	Decision   ApprovalDecision      `json:"decision,omitempty"`
	Remember   *RememberScope        `json:"remember,omitzero"`
	EditedArgs map[string]any        `json:"editedArgs,omitzero"`
	Reason     string                `json:"reason,omitempty"`
	Answers    [][]string            `json:"answers,omitempty"`
}

//flame:enum
type RememberScopeKind string

const (
	RememberSession RememberScopeKind = "session"
	RememberProject RememberScopeKind = "project"
	RememberGlobal  RememberScopeKind = "global"
)

type RememberScope struct {
	Scope RememberScopeKind `json:"scope"`
}

//flame:enum
type InterruptType string

const (
	InterruptApproval InterruptType = "approval"
	InterruptQuestion InterruptType = "question"
)

//flame:enum
type ApprovalRisk string

const (
	ApprovalRiskLow    ApprovalRisk = "low"
	ApprovalRiskMedium ApprovalRisk = "medium"
	ApprovalRiskHigh   ApprovalRisk = "high"
)

type InterruptPayload struct {
	Tool         *ToolInvocation `json:"tool,omitzero"`
	Risk         ApprovalRisk    `json:"risk,omitempty"`
	Reason       string          `json:"reason,omitempty"`
	Rememberable bool            `json:"rememberable,omitzero"`
	Question     *Question       `json:"question,omitzero"`
}

type Interrupt struct {
	ItemID  string            `json:"itemId"`
	RunID   string            `json:"runId"`
	Type    InterruptType     `json:"type"`
	Payload *InterruptPayload `json:"payload,omitzero"`
}

type PendingInterruptSet struct {
	RootRunID  string      `json:"rootRunId"`
	SessionID  string      `json:"sessionId"`
	Interrupts []Interrupt `json:"interrupts"`
	CreatedAt  time.Time   `json:"createdAt,omitzero"`
}
