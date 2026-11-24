package session

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

const MaximumCatalogSearchCharacters = 1024

type catalogFilterKind uint8

const (
	allCatalogEntries catalogFilterKind = iota + 1
	filteredCatalogEntries
)

// A filtered catalog carries at least one criterion; the closed kind keeps the
// zero value from reading as "all entries".
type CatalogFilter struct {
	kind      catalogFilterKind
	search    string
	workspace string
	project   identity.ProjectID
}

func AllCatalogEntries() CatalogFilter { return CatalogFilter{kind: allCatalogEntries} }

func NewCatalogFilter(search string, workspace *Workspace, project identity.ProjectID) (CatalogFilter, error) {
	normalizedSearch, err := NormalizeCatalogText(strings.TrimSpace(search))
	if err != nil {
		return CatalogFilter{}, fmt.Errorf("session: catalog search: %w", err)
	}
	if utf8.RuneCountInString(normalizedSearch) > MaximumCatalogSearchCharacters {
		return CatalogFilter{}, fmt.Errorf(
			"session: catalog search exceeds %d characters",
			MaximumCatalogSearchCharacters,
		)
	}
	workspacePath := ""
	if workspace != nil {
		if err := workspace.Validate(); err != nil {
			return CatalogFilter{}, fmt.Errorf("session: catalog workspace: %w", err)
		}
		workspacePath = workspace.Path()
	}
	if normalizedSearch == "" && workspacePath == "" && project.IsZero() {
		return CatalogFilter{}, errors.New("session: filtered catalog requires search, workspace or project")
	}
	return CatalogFilter{
		kind: filteredCatalogEntries, search: normalizedSearch,
		workspace: workspacePath, project: project,
	}, nil
}

func NormalizeCatalogText(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", errors.New("text is not valid UTF-8")
	}
	if strings.ContainsRune(value, 0) {
		return "", errors.New("text contains NUL")
	}
	return strings.ToLower(value), nil
}

func (f CatalogFilter) Validate() error {
	switch f.kind {
	case allCatalogEntries, filteredCatalogEntries:
		return nil
	default:
		return errors.New("session: catalog filter mode is unknown")
	}
}

func (f CatalogFilter) Search() (string, bool) { return f.search, f.search != "" }

func (f CatalogFilter) WorkspacePath() (string, bool) { return f.workspace, f.workspace != "" }

func (f CatalogFilter) Project() (identity.ProjectID, bool) { return f.project, !f.project.IsZero() }

func (f CatalogFilter) CursorIdentity() []string {
	var identity []string
	if search, ok := f.Search(); ok {
		identity = append(identity, "search", search)
	}
	if workspace, ok := f.WorkspacePath(); ok {
		identity = append(identity, "workspace", workspace)
	}
	if project, ok := f.Project(); ok {
		identity = append(identity, "project", project.String())
	}
	return identity
}

type CatalogAnchor struct {
	favorite  bool
	updatedAt time.Time
	id        identity.SessionID
}

func NewCatalogAnchor(favorite bool, updatedAt time.Time, id identity.SessionID) (CatalogAnchor, error) {
	if updatedAt.IsZero() {
		return CatalogAnchor{}, errors.New("session: catalog anchor update time is required")
	}
	if err := id.Validate(); err != nil {
		return CatalogAnchor{}, fmt.Errorf("session: catalog anchor: %w", err)
	}
	return CatalogAnchor{favorite: favorite, updatedAt: updatedAt.UTC(), id: id}, nil
}

func (a CatalogAnchor) Validate() error {
	if a.updatedAt.IsZero() {
		return errors.New("session: catalog anchor update time is required")
	}
	return nil
}

func (a CatalogAnchor) Favorite() bool         { return a.favorite }
func (a CatalogAnchor) UpdatedAt() time.Time   { return a.updatedAt }
func (a CatalogAnchor) ID() identity.SessionID { return a.id }

type CatalogRead struct {
	filter CatalogFilter
	after  *CatalogAnchor
	limit  int
}

func NewCatalogRead(filter CatalogFilter, after *CatalogAnchor, limit int) (CatalogRead, error) {
	if err := filter.Validate(); err != nil {
		return CatalogRead{}, err
	}
	if limit <= 0 {
		return CatalogRead{}, errors.New("session: catalog read limit must be greater than zero")
	}
	read := CatalogRead{filter: filter, limit: limit}
	if after != nil {
		if err := after.Validate(); err != nil {
			return CatalogRead{}, err
		}
		cloned := *after
		read.after = &cloned
	}
	return read, nil
}

func (r CatalogRead) Validate() error {
	if r.limit <= 0 {
		return errors.New("session: catalog read limit must be greater than zero")
	}
	return r.filter.Validate()
}

func (r CatalogRead) Filter() CatalogFilter { return r.filter }
func (r CatalogRead) Limit() int            { return r.limit }

func (r CatalogRead) After() (CatalogAnchor, bool) {
	if r.after == nil {
		return CatalogAnchor{}, false
	}
	return *r.after, true
}
