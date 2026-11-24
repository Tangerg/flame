package session

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
)

var (
	ErrNotFound         = errors.New("session: not found")
	ErrInvalid          = errors.New("session: invalid session")
	ErrTitleRequired    = errors.New("session: title required")
	ErrRevisionConflict = errors.New("session: revision conflict")
)

type Draft struct {
	ID        identity.SessionID
	Title     string
	Workspace Workspace
	Project   identity.ProjectID
	Selection flamemodel.Selection
	CreatedAt time.Time
}

// A zero Project in a present patch removes the Session from its Project.
type Patch struct {
	Title     *string
	Selection *flamemodel.Selection
	Workspace *Workspace
	Project   *identity.ProjectID
	Favorite  *bool
	Isolated  *bool
}

func (p Patch) Empty() bool {
	return p.Title == nil && p.Selection == nil && p.Workspace == nil && p.Project == nil && p.Favorite == nil && p.Isolated == nil
}

type Snapshot struct {
	ID        identity.SessionID
	Title     string
	Workspace Workspace
	Project   identity.ProjectID
	Selection flamemodel.Selection
	ParentID  identity.SessionID
	CreatedAt time.Time
	UpdatedAt time.Time
	Favorite  bool
	Isolated  bool
	Revision  uint64
}

type Session struct {
	id        identity.SessionID
	title     string
	workspace Workspace
	project   identity.ProjectID
	selection flamemodel.Selection
	parentID  identity.SessionID
	createdAt time.Time
	updatedAt time.Time
	favorite  bool
	isolated  bool
	revision  exactint.Counter
}

func New(draft Draft) (Session, error) {
	createdAt := draft.CreatedAt.UTC()
	snapshot := Snapshot{
		ID: draft.ID, Title: strings.TrimSpace(draft.Title),
		Workspace: draft.Workspace, Project: draft.Project, Selection: draft.Selection,
		CreatedAt: createdAt, UpdatedAt: createdAt, Revision: exactint.First().Value(),
	}
	return Restore(snapshot)
}

func Restore(snapshot Snapshot) (Session, error) {
	revision, err := exactint.Restore(snapshot.Revision)
	if err != nil {
		return Session{}, fmt.Errorf("%w: revision: %w", ErrInvalid, err)
	}
	value := Session{
		id: snapshot.ID, title: snapshot.Title, workspace: snapshot.Workspace,
		project: snapshot.Project, selection: snapshot.Selection, parentID: snapshot.ParentID,
		createdAt: snapshot.CreatedAt.UTC(),
		updatedAt: snapshot.UpdatedAt.UTC(),
		favorite:  snapshot.Favorite, isolated: snapshot.Isolated,
		revision: revision,
	}
	if err := value.validate(); err != nil {
		return Session{}, err
	}
	return value, nil
}

func (s Session) Edit(expectedRevision uint64, patch Patch, updatedAt time.Time) (Session, bool, error) {
	if !s.id.IsZero() && expectedRevision != s.revision.Value() {
		return Session{}, false, ErrRevisionConflict
	}
	return s.Apply(patch, updatedAt)
}

func (s Session) Apply(patch Patch, updatedAt time.Time) (next Session, changed bool, err error) {
	if s.id.IsZero() {
		return Session{}, false, fmt.Errorf("%w: current state was never constructed", ErrInvalid)
	}
	if patch.Empty() {
		return s, false, nil
	}

	next = s
	if patch.Title != nil {
		title := strings.TrimSpace(*patch.Title)
		if title == "" {
			return Session{}, false, ErrTitleRequired
		}
		next.title = title
	}
	if patch.Selection != nil {
		next.selection = *patch.Selection
	}
	if patch.Workspace != nil {
		if err := patch.Workspace.Validate(); err != nil {
			return Session{}, false, err
		}
		next.workspace = *patch.Workspace
	}
	if patch.Project != nil {
		next.project = *patch.Project
	}
	if patch.Favorite != nil {
		next.favorite = *patch.Favorite
	}
	if patch.Isolated != nil {
		next.isolated = *patch.Isolated
	}
	if next.sameValue(s) {
		return s, false, nil
	}
	advanced, err := next.advanced(s, updatedAt)
	if err != nil {
		return Session{}, false, err
	}
	return advanced, true, nil
}

func (s Session) NameIfUntitled(title string, updatedAt time.Time) (Session, bool, error) {
	if strings.TrimSpace(s.title) != "" {
		return s, false, nil
	}
	return s.Apply(Patch{Title: &title}, updatedAt)
}

func (s Session) Fork(id identity.SessionID, title string, createdAt time.Time) (Session, error) {
	if s.id.IsZero() {
		return Session{}, fmt.Errorf("%w: parent was never constructed", ErrInvalid)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = s.title
	}
	createdAt = createdAt.UTC()
	return Restore(Snapshot{
		ID: id, Title: title, Workspace: s.workspace, Project: s.project, ParentID: s.id,
		CreatedAt: createdAt, UpdatedAt: createdAt,
		Selection: s.selection, Isolated: s.isolated, Revision: exactint.First().Value(),
	})
}

func (s Session) InstallRestoredWorkspace(workspace Workspace) (Session, error) {
	if s.id.IsZero() {
		return Session{}, fmt.Errorf("%w: value was never constructed", ErrInvalid)
	}
	if err := workspace.Validate(); err != nil {
		return Session{}, err
	}
	s.workspace = workspace
	return s, s.validate()
}

func (s Session) ReplaceWithRestore(restored Session, updatedAt time.Time) (Session, error) {
	if s.id.IsZero() || restored.id.IsZero() {
		return Session{}, fmt.Errorf("%w: replacement needs a constructed current and restored Session", ErrInvalid)
	}
	if s.id != restored.id {
		return Session{}, fmt.Errorf("%w: restored identity %q differs from current identity %q", ErrInvalid, restored.id, s.id)
	}
	return restored.advanced(s, updatedAt)
}

func (s Session) advanced(previous Session, updatedAt time.Time) (Session, error) {
	updatedAt = updatedAt.UTC()
	if updatedAt.IsZero() {
		return Session{}, fmt.Errorf("%w: update time is required", ErrInvalid)
	}
	if updatedAt.Before(previous.updatedAt) || updatedAt.Before(s.createdAt) {
		return Session{}, fmt.Errorf("%w: update time precedes Session history", ErrInvalid)
	}
	revision, err := previous.revision.Next()
	if err != nil {
		return Session{}, fmt.Errorf("%w: revision: %w", ErrInvalid, err)
	}
	s.revision = revision
	s.updatedAt = updatedAt
	if err := s.validate(); err != nil {
		return Session{}, err
	}
	return s, nil
}

func (s Session) validate() error {
	if err := s.id.Validate(); err != nil {
		return fmt.Errorf("%w: id: %w", ErrInvalid, err)
	}
	if err := s.workspace.Validate(); err != nil {
		return err
	}
	if s.title != strings.TrimSpace(s.title) {
		return fmt.Errorf("%w: title must not contain surrounding whitespace", ErrInvalid)
	}
	if err := s.selection.ValidateExact(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if s.parentID == s.id {
		return fmt.Errorf("%w: invalid parent identity", ErrInvalid)
	}
	if s.createdAt.IsZero() || s.updatedAt.IsZero() {
		return fmt.Errorf("%w: start and update times are required", ErrInvalid)
	}
	if err := instant.Validate(s.createdAt, s.updatedAt); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if s.updatedAt.Before(s.createdAt) {
		return fmt.Errorf("%w: update time precedes start time", ErrInvalid)
	}
	if s.revision.IsZero() {
		return fmt.Errorf("%w: revision must be positive", ErrInvalid)
	}
	return nil
}

func (s Session) Snapshot() Snapshot {
	return Snapshot{
		ID: s.id, Title: s.title, Workspace: s.workspace, Project: s.project, Selection: s.selection,
		ParentID: s.parentID, CreatedAt: s.createdAt, UpdatedAt: s.updatedAt,
		Favorite: s.favorite, Isolated: s.isolated, Revision: s.revision.Value(),
	}
}

func (s Session) ID() identity.SessionID { return s.id }

func (s Session) Title() string { return s.title }

func (s Session) Workspace() Workspace { return s.workspace }

// Project is zero when the Session belongs to none.
func (s Session) Project() identity.ProjectID { return s.project }

func (s Session) Selection() flamemodel.Selection { return s.selection }

func (s Session) ParentID() identity.SessionID { return s.parentID }

func (s Session) CreatedAt() time.Time { return s.createdAt }

func (s Session) UpdatedAt() time.Time { return s.updatedAt }

func (s Session) Favorite() bool { return s.favorite }

func (s Session) Isolated() bool { return s.isolated }

func (s Session) Revision() uint64 { return s.revision.Value() }

// The workspace and isolation decide where a Session's processes run and what
// they may touch; Sessions that differ in either cannot share that state.
func (s Session) SharesExecutionPolicy(other Session) bool {
	return s.workspace == other.workspace && s.isolated == other.isolated
}

func (s Session) sameValue(other Session) bool {
	return s.id == other.id && s.title == other.title && s.workspace == other.workspace && s.project == other.project &&
		s.selection.Equal(other.selection) && s.parentID == other.parentID &&
		s.createdAt.Equal(other.createdAt) && s.favorite == other.favorite &&
		s.isolated == other.isolated
}
