package project

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
)

var (
	ErrNotFound             = errors.New("project: not found")
	ErrInvalid              = errors.New("project: invalid project")
	ErrNameRequired         = errors.New("project: name required")
	ErrInstructionsTooLarge = errors.New("project: instructions too large")
	ErrRevisionConflict     = errors.New("project: revision conflict")
)

// Instructions are sent with every Run's system message, so their size is
// bounded.
const MaxInstructionsBytes = 32 * 1024

type Draft struct {
	ID           identity.ProjectID
	Name         string
	Instructions string
	CreatedAt    time.Time
}

type Patch struct {
	Name         *string
	Instructions *string
	Archived     *bool
}

type Snapshot struct {
	ID           identity.ProjectID
	Name         string
	Instructions string
	Archived     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Revision     uint64
}

type Project struct {
	id           identity.ProjectID
	name         string
	instructions string
	archived     bool
	createdAt    time.Time
	updatedAt    time.Time
	revision     exactint.Counter
}

func New(draft Draft) (Project, error) {
	createdAt := draft.CreatedAt.UTC()
	return Restore(Snapshot{
		ID: draft.ID, Name: strings.TrimSpace(draft.Name),
		Instructions: strings.TrimSpace(draft.Instructions),
		CreatedAt:    createdAt, UpdatedAt: createdAt, Revision: exactint.First().Value(),
	})
}

func Restore(snapshot Snapshot) (Project, error) {
	revision, err := exactint.Restore(snapshot.Revision)
	if err != nil {
		return Project{}, fmt.Errorf("%w: revision: %w", ErrInvalid, err)
	}
	value := Project{
		id: snapshot.ID, name: snapshot.Name, instructions: snapshot.Instructions,
		archived: snapshot.Archived, createdAt: snapshot.CreatedAt.UTC(),
		updatedAt: snapshot.UpdatedAt.UTC(), revision: revision,
	}
	if err := value.validate(); err != nil {
		return Project{}, err
	}
	return value, nil
}

func (p Project) Edit(expectedRevision uint64, patch Patch, updatedAt time.Time) (Project, bool, error) {
	if p.id.IsZero() {
		return Project{}, false, fmt.Errorf("%w: current state was never constructed", ErrInvalid)
	}
	if expectedRevision != p.revision.Value() {
		return Project{}, false, ErrRevisionConflict
	}
	next := p
	if patch.Name != nil {
		next.name = strings.TrimSpace(*patch.Name)
	}
	if patch.Instructions != nil {
		next.instructions = strings.TrimSpace(*patch.Instructions)
	}
	if patch.Archived != nil {
		next.archived = *patch.Archived
	}
	if next.name == p.name && next.instructions == p.instructions && next.archived == p.archived {
		return p, false, nil
	}
	updatedAt = updatedAt.UTC()
	if updatedAt.Before(p.updatedAt) {
		return Project{}, false, fmt.Errorf("%w: update time precedes Project history", ErrInvalid)
	}
	revision, err := p.revision.Next()
	if err != nil {
		return Project{}, false, fmt.Errorf("%w: revision: %w", ErrInvalid, err)
	}
	next.updatedAt, next.revision = updatedAt, revision
	if err := next.validate(); err != nil {
		return Project{}, false, err
	}
	return next, true, nil
}

func (p Project) validate() error {
	if err := p.id.Validate(); err != nil {
		return fmt.Errorf("%w: id: %w", ErrInvalid, err)
	}
	if p.name == "" {
		return ErrNameRequired
	}
	if p.name != strings.TrimSpace(p.name) || p.instructions != strings.TrimSpace(p.instructions) {
		return fmt.Errorf("%w: text must not contain surrounding whitespace", ErrInvalid)
	}
	if !utf8.ValidString(p.name) || !utf8.ValidString(p.instructions) {
		return fmt.Errorf("%w: text is not valid UTF-8", ErrInvalid)
	}
	if len(p.instructions) > MaxInstructionsBytes {
		return ErrInstructionsTooLarge
	}
	if p.createdAt.IsZero() || p.updatedAt.IsZero() || p.updatedAt.Before(p.createdAt) {
		return fmt.Errorf("%w: start and update times are required in order", ErrInvalid)
	}
	if err := instant.Validate(p.createdAt, p.updatedAt); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if p.revision.IsZero() {
		return fmt.Errorf("%w: revision must be positive", ErrInvalid)
	}
	return nil
}

func (p Project) Snapshot() Snapshot {
	return Snapshot{
		ID: p.id, Name: p.name, Instructions: p.instructions, Archived: p.archived,
		CreatedAt: p.createdAt, UpdatedAt: p.updatedAt, Revision: p.revision.Value(),
	}
}

func (p Project) ID() identity.ProjectID { return p.id }

func (p Project) Name() string { return p.name }

func (p Project) Instructions() string { return p.instructions }

func (p Project) Archived() bool { return p.archived }

func (p Project) UpdatedAt() time.Time { return p.updatedAt }

func (p Project) Revision() uint64 { return p.revision.Value() }
