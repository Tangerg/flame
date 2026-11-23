package protocol

import (
	"time"

	framecall "github.com/Tangerg/frame/call"
)

type ListModelEndpointsRequest struct {
	ParentCallID string `json:"parentCallId"`
	PageQuery
}

//flame:enum
type ModelEndpointKind string

const (
	ModelEndpointChat      ModelEndpointKind = "chat"
	ModelEndpointEmbedding ModelEndpointKind = "embedding"
)

type ModelEndpointInvocation struct {
	CostUSD   *float64             `json:"costUsd,omitzero"`
	ID        string               `json:"id"`
	Kind      ModelEndpointKind    `json:"kind"`
	SessionID string               `json:"sessionId,omitzero"`
	RunID     string               `json:"runId,omitzero"`
	SegmentID string               `json:"segmentId,omitzero"`
	Caller    framecall.Invocation `json:"caller"`
	Provider  string               `json:"provider"`
	Model     string               `json:"model"`
	Request   string               `json:"request"`
	Response  string               `json:"response,omitzero"`
	Failure   string               `json:"failure,omitzero"`
	State     ModelInvocationState `json:"state"`
	StartedAt time.Time            `json:"startedAt"`
	SettledAt time.Time            `json:"settledAt,omitzero"`
}

//flame:enum
type ModelInvocationState string

const (
	ModelInvocationStarted   ModelInvocationState = "started"
	ModelInvocationCompleted ModelInvocationState = "completed"
	ModelInvocationFailed    ModelInvocationState = "failed"
	ModelInvocationUnknown   ModelInvocationState = "unknown"
)

type ModelInvocationUsage struct {
	InputTokens      int64  `json:"inputTokens"`
	OutputTokens     int64  `json:"outputTokens"`
	CacheReadTokens  *int64 `json:"cacheReadTokens,omitzero"`
	CacheWriteTokens *int64 `json:"cacheWriteTokens,omitzero"`
	ReasoningTokens  *int64 `json:"reasoningTokens,omitzero"`
}

type ModelInvocation struct {
	KnownResponse            string                `json:"knownResponse,omitzero"`
	FirstOutputLatencyMillis *int64                `json:"firstOutputLatencyMillis,omitzero"`
	Usage                    *ModelInvocationUsage `json:"usage,omitzero"`
	CallID                   string                `json:"callId"`
	RunID                    string                `json:"runId"`
	SegmentID                string                `json:"segmentId"`
	State                    ModelInvocationState  `json:"state"`
	StartedAt                time.Time             `json:"startedAt"`
	SettledAt                time.Time             `json:"settledAt,omitzero"`
}

type ListModelInvocationsRequest struct {
	RunID string `json:"runId"`
	PageQuery
}
