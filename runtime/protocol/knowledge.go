package protocol

import (
	"time"
)

// KnowledgeScope selects which FLAME.md a knowledge operation targets.
type KnowledgeScope string

const (
	KnowledgeScopeCWD         KnowledgeScope = "cwd"
	KnowledgeScopeProjectRoot KnowledgeScope = "projectRoot"
	KnowledgeScopeHome        KnowledgeScope = "home"
)

// Valid reports whether k is a known scope.
func (k KnowledgeScope) Valid() bool {
	return k == KnowledgeScopeCWD || k == KnowledgeScopeProjectRoot || k == KnowledgeScopeHome
}

// KnowledgeEntry is one knowledge record.
type KnowledgeEntry struct {
	Scope     KnowledgeScope `json:"scope"`
	Content   string         `json:"content"`
	Revision  string         `json:"revision"`
	UpdatedAt time.Time      `json:"updatedAt,omitzero"`
}

// GetKnowledgeRequest — knowledge.get body.
type GetKnowledgeRequest struct {
	Scope     KnowledgeScope `json:"scope"`
	Workspace *WorkspaceRef  `json:"workspace,omitempty"`
}

// UpdateKnowledgeRequest — knowledge.update body.
type UpdateKnowledgeRequest struct {
	Scope            KnowledgeScope `json:"scope"`
	Workspace        *WorkspaceRef  `json:"workspace,omitempty"`
	ExpectedRevision string         `json:"expectedRevision"`
	Content          string         `json:"content"`
}
