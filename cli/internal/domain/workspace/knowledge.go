package workspace

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type KnowledgeScope string

const (
	KnowledgeWorkingDirectory KnowledgeScope = "cwd"
	KnowledgeProjectRoot      KnowledgeScope = "projectRoot"
	KnowledgeHome             KnowledgeScope = "home"
)

func ParseKnowledgeScope(value string) (KnowledgeScope, error) {
	scope := KnowledgeScope(strings.TrimSpace(value))
	if err := scope.Validate(); err != nil {
		return "", err
	}
	return scope, nil
}

func (s KnowledgeScope) Validate() error {
	if s != KnowledgeWorkingDirectory && s != KnowledgeProjectRoot && s != KnowledgeHome {
		return fmt.Errorf("knowledge scope must be cwd, projectRoot, or home, got %q", s)
	}
	return nil
}

// KnowledgeTarget identifies exactly one document. Home is runtime-global and therefore
// intentionally carries no workspace; the other scopes cannot resolve without it.
type KnowledgeTarget struct {
	Scope     KnowledgeScope
	Workspace string
}

func NewKnowledgeTarget(scope KnowledgeScope, workspace string) (KnowledgeTarget, error) {
	target := KnowledgeTarget{Scope: scope, Workspace: strings.TrimSpace(workspace)}
	return target, target.Validate()
}

func (t KnowledgeTarget) Validate() error {
	if err := t.Scope.Validate(); err != nil {
		return err
	}
	if t.Scope == KnowledgeHome {
		if t.Workspace != "" {
			return errors.New("home knowledge does not belong to a workspace")
		}
		return nil
	}
	if t.Workspace == "" {
		return fmt.Errorf("%s knowledge requires a workspace", t.Scope)
	}
	return nil
}

type KnowledgeEntry struct {
	Scope     KnowledgeScope
	Content   string
	Revision  string
	UpdatedAt *time.Time
}

func (e KnowledgeEntry) Validate() error {
	if err := e.Scope.Validate(); err != nil {
		return err
	}
	if e.UpdatedAt != nil && e.UpdatedAt.IsZero() {
		return errors.New("knowledge update time is zero")
	}
	if strings.TrimSpace(e.Revision) == "" {
		return errors.New("knowledge revision is empty")
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
	if err := u.Target.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(u.ExpectedRevision) == "" {
		return errors.New("knowledge update expected revision is empty")
	}
	return nil
}
