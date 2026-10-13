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
