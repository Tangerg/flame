package session

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

// MaximumCatalogSearchCharacters bounds one user-facing session catalog
// predicate before it reaches cursor material or persistence. The limit is in
// Unicode code points, matching the public JSON Schema maxLength contract.
const MaximumCatalogSearchCharacters = 1024

type catalogFilterKind uint8

const (
	allCatalogEntries catalogFilterKind = iota + 1
	searchCatalogEntries
	workspaceCatalogEntries
	searchWorkspaceCatalogEntries
)

// CatalogFilter is the normalized identity of one sessions.list collection.
// Search is case-insensitive literal containment over durable title/workspace
// text; Workspace is an exact admitted Session workspace identity.
type CatalogFilter struct {
	kind      catalogFilterKind
	search    string
	workspace string
}

// AllCatalogEntries explicitly selects the unfiltered Session collection.
func AllCatalogEntries() CatalogFilter { return CatalogFilter{kind: allCatalogEntries} }

// NewCatalogFilter constructs a non-empty filtered collection. A caller maps
// absent predicates to AllCatalogEntries explicitly; accepting both fields
// empty here would restore an implicit zero/default mode.
func NewCatalogFilter(search string, workspace *Workspace) (CatalogFilter, error) {
	normalizedSearch, err := NormalizeCatalogText(strings.TrimSpace(search))
	if err != nil {
		return CatalogFilter{}, fmt.Errorf("sessions: catalog search: %w", err)
	}
	if utf8.RuneCountInString(normalizedSearch) > MaximumCatalogSearchCharacters {
		return CatalogFilter{}, fmt.Errorf(
			"sessions: catalog search exceeds %d characters",
			MaximumCatalogSearchCharacters,
		)
	}
	workspacePath := ""
	if workspace != nil {
		if err := workspace.Validate(); err != nil {
			return CatalogFilter{}, fmt.Errorf("sessions: catalog workspace: %w", err)
		}
		workspacePath = workspace.Path()
	}

	switch {
	case normalizedSearch != "" && workspacePath != "":
		return CatalogFilter{kind: searchWorkspaceCatalogEntries, search: normalizedSearch, workspace: workspacePath}, nil
	case normalizedSearch != "":
		return CatalogFilter{kind: searchCatalogEntries, search: normalizedSearch}, nil
	case workspacePath != "":
		return CatalogFilter{kind: workspaceCatalogEntries, workspace: workspacePath}, nil
	default:
		return CatalogFilter{}, errors.New("sessions: filtered catalog requires search or workspace")
	}
}

// NormalizeCatalogText is the sole canonicalizer shared by query admission and
// durable Session search material. Keeping persisted Unicode lowercase text in
// this exact form avoids SQLite's ASCII-only lower() behavior.
func NormalizeCatalogText(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", errors.New("text is not valid UTF-8")
	}
	if strings.ContainsRune(value, 0) {
		return "", errors.New("text contains NUL")
	}
	return strings.ToLower(value), nil
}

// Validate rejects zero/corrupt filter state at every read boundary.
func (f CatalogFilter) Validate() error {
	switch f.kind {
	case allCatalogEntries:
		if f.search != "" || f.workspace != "" {
			return errors.New("sessions: all-catalog filter carries predicates")
		}
		return nil
	case searchCatalogEntries, workspaceCatalogEntries, searchWorkspaceCatalogEntries:
		var workspace *Workspace
		if f.workspace != "" {
			value, err := NewWorkspace(f.workspace)
			if err != nil {
				return err
			}
			workspace = &value
		}
		rebuilt, err := NewCatalogFilter(f.search, workspace)
		if err != nil {
			return err
		}
		if rebuilt != f {
			return errors.New("sessions: catalog filter is not canonical")
		}
		return nil
	default:
		return errors.New("sessions: catalog filter mode is unknown")
	}
}

func (f CatalogFilter) Search() (string, bool) {
	return f.search, f.kind == searchCatalogEntries || f.kind == searchWorkspaceCatalogEntries
}

func (f CatalogFilter) WorkspacePath() (string, bool) {
	return f.workspace, f.kind == workspaceCatalogEntries || f.kind == searchWorkspaceCatalogEntries
}

// CursorIdentity is the fixed-field query identity framed into every keyset
// token. An unfiltered query keeps the historical nil filter identity.
func (f CatalogFilter) CursorIdentity() []string {
	switch f.kind {
	case allCatalogEntries:
		return nil
	case searchCatalogEntries:
		return []string{"search", f.search}
	case workspaceCatalogEntries:
		return []string{"workspace", f.workspace}
	case searchWorkspaceCatalogEntries:
		return []string{"search", f.search, "workspace", f.workspace}
	default:
		return nil
	}
}

// CatalogAnchor is the complete favorite/recency/id key of the last Session in
// a page. Pointer presence in CatalogRead distinguishes the first page; no
// primitive timestamp or id sentinel crosses into persistence.
type CatalogAnchor struct {
	favorite  bool
	updatedAt time.Time
	id        string
}

func NewCatalogAnchor(favorite bool, updatedAt time.Time, id string) (CatalogAnchor, error) {
	if updatedAt.IsZero() {
		return CatalogAnchor{}, errors.New("sessions: catalog anchor update time is required")
	}
	if _, err := resourceid.ParseSession(id); err != nil {
		return CatalogAnchor{}, fmt.Errorf("sessions: catalog anchor: %w", err)
	}
	return CatalogAnchor{favorite: favorite, updatedAt: updatedAt.UTC(), id: id}, nil
}

func (a CatalogAnchor) Validate() error {
	rebuilt, err := NewCatalogAnchor(a.favorite, a.updatedAt, a.id)
	if err != nil {
		return err
	}
	if rebuilt != a {
		return errors.New("sessions: catalog anchor is not canonical")
	}
	return nil
}

func (a CatalogAnchor) Favorite() bool       { return a.favorite }
func (a CatalogAnchor) UpdatedAt() time.Time { return a.updatedAt }
func (a CatalogAnchor) ID() string           { return a.id }

// CatalogRead is the complete persistence request for one bounded keyset read.
type CatalogRead struct {
	filter CatalogFilter
	after  *CatalogAnchor
	limit  int
}

// ErrInvalidCatalogPage reports store output that does not belong to the exact
// bounded collection described by a CatalogRead.
var ErrInvalidCatalogPage = errors.New("sessions: invalid catalog page")

func NewCatalogRead(filter CatalogFilter, after *CatalogAnchor, limit int) (CatalogRead, error) {
	if err := filter.Validate(); err != nil {
		return CatalogRead{}, err
	}
	if limit <= 0 {
		return CatalogRead{}, errors.New("sessions: catalog read limit must be greater than zero")
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
	rebuilt, err := NewCatalogRead(r.filter, r.after, r.limit)
	if err != nil {
		return err
	}
	if rebuilt.filter != r.filter || rebuilt.limit != r.limit ||
		(rebuilt.after == nil) != (r.after == nil) ||
		(rebuilt.after != nil && *rebuilt.after != *r.after) {
		return errors.New("sessions: catalog read is not canonical")
	}
	return nil
}

func (r CatalogRead) Filter() CatalogFilter { return r.filter }
func (r CatalogRead) Limit() int            { return r.limit }

func (r CatalogRead) After() (CatalogAnchor, bool) {
	if r.after == nil {
		return CatalogAnchor{}, false
	}
	return *r.after, true
}

// ValidateCatalog checks one complete unfiltered Session catalog. Complete reads
// have no overfetch ceiling, but they share the page contract's aggregate,
// identity, and favorite/recency order invariants.
func ValidateCatalog(values []Session) error {
	read, err := NewCatalogRead(AllCatalogEntries(), nil, max(1, len(values)))
	if err != nil {
		return err
	}
	return read.ValidatePage(values)
}

// ValidatePage checks a store page before Application resolves live state or
// mints a continuation from it. Store order is favorite first, then update time
// and Session identity descending; every returned value must strictly follow
// both the cursor anchor and its predecessor.
func (r CatalogRead) ValidatePage(values []Session) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCatalogPage, err)
	}
	if len(values) > r.limit {
		return fmt.Errorf("%w: store returned %d rows, maximum %d", ErrInvalidCatalogPage, len(values), r.limit)
	}
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if err := value.Validate(); err != nil {
			return fmt.Errorf("%w: row %d: %w", ErrInvalidCatalogPage, index+1, err)
		}
		matches, err := r.filter.matches(value)
		if err != nil {
			return fmt.Errorf("%w: row %d search material: %w", ErrInvalidCatalogPage, index+1, err)
		}
		if !matches {
			return fmt.Errorf("%w: Session %q does not match the catalog filter", ErrInvalidCatalogPage, value.ID())
		}
		if _, duplicate := seen[value.ID()]; duplicate {
			return fmt.Errorf("%w: page repeats Session %q", ErrInvalidCatalogPage, value.ID())
		}
		seen[value.ID()] = struct{}{}
		if r.after != nil && !value.followsCatalogPosition(r.after.favorite, r.after.updatedAt, r.after.id) {
			return fmt.Errorf("%w: Session %q does not follow the cursor", ErrInvalidCatalogPage, value.ID())
		}
		if index > 0 {
			previous := values[index-1]
			if !value.followsCatalogPosition(previous.favorite, previous.updatedAt, previous.id) {
				return fmt.Errorf("%w: Session %q is out of order after %q", ErrInvalidCatalogPage, value.ID(), previous.ID())
			}
		}
	}
	return nil
}

func (f CatalogFilter) matches(value Session) (bool, error) {
	if workspace, present := f.WorkspacePath(); present && value.workspace.Path() != workspace {
		return false, nil
	}
	search, present := f.Search()
	if !present {
		return true, nil
	}
	title, err := NormalizeCatalogText(value.title)
	if err != nil {
		return false, err
	}
	workspace, err := NormalizeCatalogText(value.workspace.Path())
	if err != nil {
		return false, err
	}
	return strings.Contains(title, search) || strings.Contains(workspace, search), nil
}

func (s Session) followsCatalogPosition(favorite bool, updatedAt time.Time, id string) bool {
	if s.favorite != favorite {
		return favorite && !s.favorite
	}
	if !s.updatedAt.Equal(updatedAt) {
		return s.updatedAt.Before(updatedAt)
	}
	return s.id < id
}
