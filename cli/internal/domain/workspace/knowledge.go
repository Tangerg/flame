package workspace

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"
)

func ParseKnowledgeScope(value string) (protocol.KnowledgeScope, error) {
	scope := protocol.KnowledgeScope(strings.TrimSpace(value))
	if !scope.Valid() {
		return "", fmt.Errorf("knowledge scope must be cwd, projectRoot, or home, got %q", scope)
	}
	return scope, nil
}

// KnowledgeTarget identifies exactly one document. Home is runtime-global and therefore
// intentionally carries no workspace; the other scopes cannot resolve without it.
type KnowledgeTarget struct {
	Scope     protocol.KnowledgeScope
	Workspace string
}

func NewKnowledgeTarget(scope protocol.KnowledgeScope, workspace string) (KnowledgeTarget, error) {
	target := KnowledgeTarget{Scope: scope, Workspace: strings.TrimSpace(workspace)}
	return target, target.Validate()
}

func (t KnowledgeTarget) Validate() error {
	return (protocol.GetKnowledgeRequest{
		Scope: t.Scope, Workspace: t.workspaceRef(),
	}).ValidateWire()
}

func (t KnowledgeTarget) workspaceRef() *protocol.WorkspaceRef {
	if t.Workspace == "" {
		return nil
	}
	return &protocol.WorkspaceRef{Path: t.Workspace}
}

type KnowledgeEntry struct {
	Scope     protocol.KnowledgeScope
	Content   string
	Revision  string
	UpdatedAt *time.Time
}

func (e KnowledgeEntry) Validate() error {
	wire := protocol.KnowledgeEntry{Scope: e.Scope, Content: e.Content, Revision: e.Revision}
	if e.UpdatedAt != nil {
		wire.UpdatedAt = *e.UpdatedAt
	}
	if err := wire.ValidateWire(); err != nil {
		return err
	}
	if e.UpdatedAt != nil && e.UpdatedAt.IsZero() {
		return errors.New("knowledge update time is zero")
	}
	return nil
}

// Revise binds edited content to the exact document version the user opened.
// The runtime can therefore reject a stale editor instead of overwriting a
// concurrent change.
func (e KnowledgeEntry) Revise(target KnowledgeTarget, content string) (KnowledgeUpdate, error) {
	if err := e.Validate(); err != nil {
		return KnowledgeUpdate{}, err
	}
	if err := target.Validate(); err != nil {
		return KnowledgeUpdate{}, err
	}
	if e.Scope != target.Scope {
		return KnowledgeUpdate{}, fmt.Errorf("knowledge entry scope %s does not match target %s", e.Scope, target.Scope)
	}
	return KnowledgeUpdate{Target: target, ExpectedRevision: e.Revision, Content: content}, nil
}

type KnowledgeUpdate struct {
	Target           KnowledgeTarget
	ExpectedRevision string
	Content          string
}

func (u KnowledgeUpdate) Validate() error {
	return (protocol.UpdateKnowledgeRequest{
		Scope: u.Target.Scope, Workspace: u.Target.workspaceRef(),
		ExpectedRevision: u.ExpectedRevision, Content: u.Content,
	}).ValidateWire()
}
