package agent

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// MemoryScope is the durable partition that owns a memory item.
type MemoryScope string

const (
	MemoryProject MemoryScope = "project"
	MemoryUser    MemoryScope = "user"
)

func ParseMemoryScope(value string) (MemoryScope, error) {
	scope := MemoryScope(strings.TrimSpace(value))
	if err := scope.Validate(); err != nil {
		return "", err
	}
	return scope, nil
}

func (s MemoryScope) Validate() error {
	if s != MemoryProject && s != MemoryUser {
		return fmt.Errorf("agent memory scope must be project or user, got %q", s)
	}
	return nil
}

// MemoryTarget couples a scope to exactly the workspace context it requires.
type MemoryTarget struct {
	Scope     MemoryScope
	Workspace string
}

func NewMemoryTarget(scope MemoryScope, workspace string) (MemoryTarget, error) {
	target := MemoryTarget{Scope: scope, Workspace: strings.TrimSpace(workspace)}
	return target, target.Validate()
}

func (t MemoryTarget) Validate() error {
	if err := t.Scope.Validate(); err != nil {
		return err
	}
	switch t.Scope {
	case MemoryProject:
		if t.Workspace == "" {
			return errors.New("project agent memory requires a workspace")
		}
		if !filepath.IsAbs(t.Workspace) {
			return errors.New("project agent memory workspace is not absolute")
		}
	case MemoryUser:
		if t.Workspace != "" {
			return errors.New("user agent memory does not belong to a workspace")
		}
	}
	return nil
}

type MemoryOrigin string

const (
	MemoryAutomatic MemoryOrigin = "auto"
	MemoryAuthored  MemoryOrigin = "user"
)

func (o MemoryOrigin) Validate() error {
	if o != MemoryAutomatic && o != MemoryAuthored {
		return fmt.Errorf("unknown agent memory origin %q", o)
	}
	return nil
}

type MemoryStatus string

const (
	MemoryActive  MemoryStatus = "active"
	MemoryPending MemoryStatus = "pending"
)

func (s MemoryStatus) Validate() error {
	if s != MemoryActive && s != MemoryPending {
		return fmt.Errorf("unknown agent memory status %q", s)
	}
	return nil
}

// MemoryItem is one stable, addressable fact together with its review provenance.
type MemoryItem struct {
	ID        string
	Scope     MemoryScope
	Content   string
	Origin    MemoryOrigin
	Status    MemoryStatus
	Pinned    bool
	SessionID string
	Day       string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (i MemoryItem) Validate() error {
	if strings.TrimSpace(i.ID) == "" {
		return errors.New("agent memory item id is empty")
	}
	if err := i.Scope.Validate(); err != nil {
		return fmt.Errorf("agent memory item %s: %w", i.ID, err)
	}
	if strings.TrimSpace(i.Content) == "" {
		return fmt.Errorf("agent memory item %s has empty content", i.ID)
	}
	if err := i.Origin.Validate(); err != nil {
		return fmt.Errorf("agent memory item %s: %w", i.ID, err)
	}
	if err := i.Status.Validate(); err != nil {
		return fmt.Errorf("agent memory item %s: %w", i.ID, err)
	}
	if i.Origin == MemoryAuthored && i.Status != MemoryActive {
		return fmt.Errorf("agent memory item %s: user-authored memory must be active", i.ID)
	}
	if i.CreatedAt.IsZero() || i.UpdatedAt.IsZero() {
		return fmt.Errorf("agent memory item %s has incomplete timestamps", i.ID)
	}
	if i.UpdatedAt.Before(i.CreatedAt) {
		return fmt.Errorf("agent memory item %s was updated before creation", i.ID)
	}
	return nil
}

type MemoryReviewDecision string

const (
	MemoryApprove MemoryReviewDecision = "approve"
	MemoryReject  MemoryReviewDecision = "reject"
)

func (r MemoryReviewDecision) Validate() error {
	if r != MemoryApprove && r != MemoryReject {
		return fmt.Errorf("agent memory review decision must be approve or reject, got %q", r)
	}
	return nil
}

// MemoryPatch changes one item without manufacturing a default for an omitted field.
type MemoryPatch struct {
	ID      string
	Content *string
	Pinned  *bool
}

func (p MemoryPatch) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return errors.New("agent memory patch id is empty")
	}
	if p.Content == nil && p.Pinned == nil {
		return errors.New("agent memory patch has no changes")
	}
	if p.Content != nil && strings.TrimSpace(*p.Content) == "" {
		return errors.New("agent memory content is empty")
	}
	return nil
}

func (p MemoryPatch) ValidateResult(result MemoryItem) error {
	if err := p.Validate(); err != nil {
		return err
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.ID != p.ID {
		problems = append(problems, fmt.Errorf("runtime returned item %q, want %q", result.ID, p.ID))
	}
	if p.Content != nil && result.Content != strings.TrimSpace(*p.Content) {
		problems = append(problems, fmt.Errorf("runtime returned content %q, want %q", result.Content, strings.TrimSpace(*p.Content)))
	}
	if p.Pinned != nil && result.Pinned != *p.Pinned {
		problems = append(problems, fmt.Errorf("runtime returned pinned %t, want %t", result.Pinned, *p.Pinned))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("agent memory patch: %w", err)
	}
	return nil
}

func (t MemoryTarget) ValidateAddResult(content string, result MemoryItem) error {
	if err := t.Validate(); err != nil {
		return err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("add agent memory: content is empty")
	}
	var problems []error
	if err := result.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.Scope != t.Scope {
		problems = append(problems, fmt.Errorf("runtime returned %s scope, want %s", result.Scope, t.Scope))
	}
	if result.Content != content {
		problems = append(problems, fmt.Errorf("runtime returned content %q, want %q", result.Content, content))
	}
	if result.Origin != MemoryAuthored || result.Status != MemoryActive {
		problems = append(problems, fmt.Errorf(
			"runtime returned %s/%s provenance, want %s/%s",
			result.Origin, result.Status, MemoryAuthored, MemoryActive,
		))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("add agent memory: %w", err)
	}
	return nil
}

type MemoryService interface {
	Items(context.Context, MemoryTarget) ([]MemoryItem, error)
	Review(context.Context, string, MemoryReviewDecision) error
	Update(context.Context, MemoryPatch) (MemoryItem, error)
	Delete(context.Context, string) error
	Add(context.Context, MemoryTarget, string) (MemoryItem, error)
}
