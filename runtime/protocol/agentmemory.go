package protocol

import (
	"time"
)

// AgentMemoryItem is one addressable memory item. Status is
// active | pending; origin is auto (mined) | user (authored).
type AgentMemoryItem struct {
	ID        string            `json:"id"`
	Scope     AgentMemoryScope  `json:"scope"`
	Content   string            `json:"content"`
	Origin    AgentMemoryOrigin `json:"origin"`
	Status    AgentMemoryStatus `json:"status"`
	Pinned    bool              `json:"pinned"`
	SessionID string            `json:"sessionId,omitempty"`
	Day       string            `json:"day,omitempty"`
	CreatedAt time.Time         `json:"createdAt,omitzero"`
	UpdatedAt time.Time         `json:"updatedAt,omitzero"`
}

// AgentMemoryScope determines which memory set a request or item belongs to.
type AgentMemoryScope string

const (
	AgentMemoryScopeProject AgentMemoryScope = "project"
	AgentMemoryScopeUser    AgentMemoryScope = "user"
)

// AgentMemoryOrigin records whether an item was mined or user-authored.
type AgentMemoryOrigin string

const (
	AgentMemoryOriginAuto AgentMemoryOrigin = "auto"
	AgentMemoryOriginUser AgentMemoryOrigin = "user"
)

// AgentMemoryStatus is the review state visible to a client. Rejected items
// are intentional hidden tombstones and are never projected onto this API.
type AgentMemoryStatus string

const (
	AgentMemoryStatusActive  AgentMemoryStatus = "active"
	AgentMemoryStatusPending AgentMemoryStatus = "pending"
)

// AgentMemoryReviewRequest — agentMemory.review body. decision is
// "approve" | "reject".
type AgentMemoryReviewRequest struct {
	ID       string                    `json:"id"`
	Decision AgentMemoryReviewDecision `json:"decision"`
}

// AgentMemoryReviewDecision is the explicit user verdict for a pending fact.
type AgentMemoryReviewDecision string

const (
	AgentMemoryReviewApprove AgentMemoryReviewDecision = "approve"
	AgentMemoryReviewReject  AgentMemoryReviewDecision = "reject"
)

// AgentMemoryUpdateRequest — agentMemory.update body. A nil field is unchanged.
type AgentMemoryUpdateRequest struct {
	ID      string  `json:"id"`
	Content *string `json:"content,omitzero"`
	Pinned  *bool   `json:"pinned,omitzero"`
}

// AgentMemoryAddRequest — agentMemory.add body.
type AgentMemoryAddRequest struct {
	Scope     AgentMemoryScope `json:"scope"`
	Workspace *WorkspaceRef    `json:"workspace,omitzero"`
	Content   string           `json:"content"`
}
